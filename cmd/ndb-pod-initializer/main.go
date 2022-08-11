// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mysql/ndb-operator/config/debug"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/mgmapi"
	"github.com/mysql/ndb-operator/pkg/resources/statefulset"
)

// isDnsUpdated checks if the DNS can resolve the
// current pod hostname to the right IP address.
func isDnsUpdated(ctx context.Context, hostname, expectedIP string) bool {
	resolvedIPs, err := net.DefaultResolver.LookupIP(ctx, "ip4", hostname)

	if err != nil {
		var dnsError *net.DNSError
		if errors.As(err, &dnsError) && (dnsError.IsNotFound || dnsError.IsTemporary) {
			// DNS doesn't have the entry yet
			return false
		} else {
			// Unrecognised error
			log.Fatalf("Failed to resolve hostname %q : %s", hostname, err)
		}
	}

	if len(resolvedIPs) > 1 {
		// Hostname resolved to more than 1 IP address => some other
		// pod's hostname is still pointed to this IP address in DNS.
		return false
	}

	if resolvedIPs[0].String() != expectedIP {
		// Hostname resolved to wrong IP => DNS not updated yet
		return false
	}

	return true
}

// waitForPodDNSUpdate waits until the DNS can resolve the current hostname without any error.
//
// Note 1 : This is required as the K8s CoreDNS runs with a default cache time of 30s for
// all responses. When the first Management Server comes up, it tries to resolve all the
// hostnames mentioned in the config and fails as the pods are not up yet. All these
// negative responses end up getting cached in the CoreDNS server and will be invalidated
// only when the cache time expires. The issue is explained at
// https://github.com/kubernetes/kubernetes/issues/92559. Although a fix that disables the
// caching for negative responses is now available at
// https://github.com/coredns/coredns/pull/5540, it will only be included in the latest
// versions of K8s.
//
// Note 2 : Most K8s clusters run with multiple CoreDNS replicas. There is a good chance
// that an address that resolves at one replica might not resolve at other due to the race
// caused by this cache timeout issue. The DNS queries are usually sent to a CoreDNS
// ClusterIP Service that in turn forwards the requests to the CoreDNS pods either in a
// round-robin or a random fashion. To make sure that this method doesn't return before
// the given hostname can be resolved correctly by all the CoreDNS replicas, it waits until
// the hostname can be resolved successfully for a continuous period of time. This is a
// temporary workaround and the best solution (TODO:) is to implement and use a minimal DNS
// Server that can run with the operator and resolve the hostnames of MySQL Cluster nodes
// without any delay when requested. The pods can then use it to resolve the other pod
// hostnames and the CoreDNS server for everything else.
func waitForPodDNSUpdate(ctx context.Context, podHostname string) {
	podNamespace := os.Getenv("NDB_POD_NAMESPACE")

	// Use partial fqdn so that the query gets sent to DNS.
	hostnameToResolve := fmt.Sprintf("%s.%s.%s", podHostname, podHostname[:len(podHostname)-2], podNamespace)

	podIP := os.Getenv("NDB_POD_IP")

	// Record the time the last failure occurred
	lastFailureTime := time.Now()

	log.Println("Waiting for Pod's hostname to be updated in DNS...")
	for {
		if isDnsUpdated(ctx, hostnameToResolve, podIP) {
			// Consider the hostname resolvable only when all the queries sent
			// in the last 5 seconds has succeeded.
			if time.Since(lastFailureTime) > 5*time.Second {
				log.Println("Done")
				break
			}
		} else {
			lastFailureTime = time.Now()
		}
		// DNS not updated yet - sleep and retry
		time.Sleep(100 * time.Millisecond)
	}
}

// getPodMySQLClusterNodeType returns the type of the MySQL Cluster node the pod is running.
func getPodMySQLClusterNodeType(podHostname string) constants.NdbNodeType {
	// Hostname will be of form <ndbcluster name>-<node-type>-<sfset pod ordinal index>
	tokens := strings.Split(podHostname, "-")
	return tokens[len(tokens)-2]
}

// writeNodeIDToFile deduces the nodeId of the current MySQL Cluster
// node, writes it to a file and then returns the nodeId.
func writeNodeIDToFile(hostname string) (nodeId int, nodeType mgmapi.NodeTypeEnum) {
	// All nodeIds are sequentially assigned based on the ordinal indices
	// of the StatefulSet pods. So deduce the nodeId of the first MySQL
	// Cluster node of same nodeType (i.e) the node with StatefulSet ordinal
	// index 0, of same type. Then the nodeId of the current node will be
	// startNodeId + pod ordinal index.
	var startNodeIdOfSameNodeType int
	switch getPodMySQLClusterNodeType(hostname) {
	case constants.NdbNodeTypeMgmd:
		// Management nodes have nodeId 1 and 2.
		startNodeIdOfSameNodeType = 1
		nodeType = mgmapi.NodeTypeMGM
	case constants.NdbNodeTypeNdbmtd:
		// Data nodes' nodeId continue sequentially
		// after the Management nodes' nodeIds.
		numOfManagementNode := len(strings.Split(os.Getenv("NDB_CONNECTSTRING"), ","))
		startNodeIdOfSameNodeType = numOfManagementNode + 1
		nodeType = mgmapi.NodeTypeNDB
	case constants.NdbNodeTypeMySQLD:
		startNodeIdOfSameNodeType = constants.NdbNodeTypeAPIStartNodeId
		nodeType = mgmapi.NodeTypeAPI
	}

	// Extract pod ordinal index from hostname.
	// Hostname will be of form <ndbcluster name>-<node-type>-<sfset pod ordinal index>
	tokens := strings.Split(hostname, "-")
	podOrdinalIndex, _ := strconv.ParseInt(tokens[len(tokens)-1], 10, 32)

	// Calculate nodeId
	nodeId = startNodeIdOfSameNodeType + int(podOrdinalIndex)

	// Persist the nodeId into a file to be used by other scripts/commands
	f, err := os.OpenFile(statefulset.NodeIdFilePath,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Failed to create file %q : %s", statefulset.NodeIdFilePath, err)
	}

	if _, err = f.WriteString(fmt.Sprintf("%d", nodeId)); err != nil {
		log.Fatalf("Failed to write nodeId to file : %s", err)
	}

	return nodeId, nodeType
}

// waitForNodeIdAvailability waits until the given nodeId can be
// successfully allocated to the MySQL Cluster node that will be run in this pod.
func waitForNodeIdAvailability(nodeId int, nodeType mgmapi.NodeTypeEnum) {
	connectstring := os.Getenv("NDB_CONNECTSTRING")
	mgmClient, err := mgmapi.NewMgmClient(connectstring)
	if err != nil {
		log.Fatalf("Failed to connect to management server : %s", err.Error())
	}

	for {
		if _, err = mgmClient.TryReserveNodeId(nodeId, nodeType); err == nil {
			// alloc nodeId succeeded => MySQL Cluster is ready to accept this connection
			break
		}

		// TryReserveNodeId failed
		if debug.Enabled {
			log.Printf("TryReserveNodeId failed : %s", err.Error())
		}
		// sleep and retry
		time.Sleep(100 * time.Millisecond)
	}

}

func main() {

	ctx := context.Background()
	podHostname := os.Getenv("HOSTNAME")

	// Wait for the hostname to be updated in the DNS
	waitForPodDNSUpdate(ctx, podHostname)

	// Persist the nodeId into a file for the other scripts/commands to use
	nodeId, nodeType := writeNodeIDToFile(podHostname)

	if nodeType != mgmapi.NodeTypeMGM {
		// Wait for the nodeId to become available for NDB and API nodes
		waitForNodeIdAvailability(nodeId, nodeType)
	}
}
