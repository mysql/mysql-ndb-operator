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

	"github.com/mysql/ndb-operator/pkg/constants"
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

// waitForPodDNSUpdate waits until the DNS can resolve the current hostname without any error
func waitForPodDNSUpdate(ctx context.Context, podHostname string) {
	podNamespace := os.Getenv("NDB_POD_NAMESPACE")

	// Use partial fqdn so that the query gets sent to DNS.
	hostnameToResolve := fmt.Sprintf("%s.%s.%s", podHostname, podHostname[:len(podHostname)-2], podNamespace)

	podIP := os.Getenv("NDB_POD_IP")

	log.Println("Waiting for Pod's hostname to be updated in DNS...")
	for {
		if isDnsUpdated(ctx, hostnameToResolve, podIP) {
			log.Println("Done")
			break
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

func writeNodeIDToFile(hostname string) {
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
	case constants.NdbNodeTypeNdbmtd:
		// Data nodes' nodeId continue sequentially
		// after the Management nodes' nodeIds.
		numOfManagementNode := len(strings.Split(os.Getenv("NDB_CONNECTSTRING"), ","))
		startNodeIdOfSameNodeType = numOfManagementNode + 1
	case constants.NdbNodeTypeMySQLD:
		startNodeIdOfSameNodeType = constants.NdbNodeTypeAPIStartNodeId
	}

	// Extract pod ordinal index from hostname.
	// Hostname will be of form <ndbcluster name>-<node-type>-<sfset pod ordinal index>
	tokens := strings.Split(hostname, "-")
	podOrdinalIndex, _ := strconv.ParseInt(tokens[len(tokens)-1], 10, 32)

	// Calculate nodeId
	nodeId := startNodeIdOfSameNodeType + int(podOrdinalIndex)

	// Persist the nodeId into a file to be used by other scripts/commands
	f, err := os.OpenFile(statefulset.NodeIdFilePath,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Failed to create file %q : %s", statefulset.NodeIdFilePath, err)
	}

	if _, err = f.WriteString(fmt.Sprintf("%d", nodeId)); err != nil {
		log.Fatalf("Failed to write nodeId to file : %s", err)
	}
}

func main() {

	ctx := context.Background()
	podHostname := os.Getenv("HOSTNAME")

	// Wait for the hostname to be updated in the DNS
	waitForPodDNSUpdate(ctx, podHostname)

	// Persist the nodeId into a file for the other scripts/commands to use
	writeNodeIDToFile(podHostname)
}
