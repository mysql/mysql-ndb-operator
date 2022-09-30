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
	"github.com/mysql/ndb-operator/pkg/mysqlclient"
	"github.com/mysql/ndb-operator/pkg/resources/statefulset"
)

// helper function to handle fatal errors
func failOnError(err error, errMsgFmt string, errMsgArgs ...interface{}) {
	if err != nil {
		log.Fatalf(errMsgFmt, errMsgArgs...)
	}
}

func isAllowedDNSError(dnsError *net.DNSError) bool {

	if dnsError.IsNotFound || dnsError.IsTemporary {
		return true
	}

	allowedDNSErrors := []string{
		"server misbehaving",
	}

	for _, allowedErr := range allowedDNSErrors {
		if strings.Contains(dnsError.Error(), allowedErr) {
			return true
		}
	}

	return false
}

// isDnsUpdated checks if the DNS can resolve the
// current pod hostname to the right IP address.
func isDnsUpdated(ctx context.Context, hostname, expectedIP string) bool {
	resolvedIPs, err := net.DefaultResolver.LookupIP(ctx, "ip4", hostname)

	if err != nil {
		var dnsError *net.DNSError
		if errors.As(err, &dnsError) && isAllowedDNSError(dnsError) {
			// DNS doesn't have the entry yet
			return false
		} else {
			// Unrecognised error
			failOnError(err, "Failed to resolve hostname %q : %s", hostname, err)
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
func writeNodeIDToFile(hostname, ndbConnectString string) (nodeId int, nodeIdPool []int, nodeType mgmapi.NodeTypeEnum) {
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
		numOfManagementNode := len(strings.Split(ndbConnectString, ",")) - 1
		startNodeIdOfSameNodeType = numOfManagementNode + 1
		nodeType = mgmapi.NodeTypeNDB
	case constants.NdbNodeTypeMySQLD:
		nodeType = mgmapi.NodeTypeAPI
	}

	// Extract pod ordinal index from hostname.
	// Hostname will be of form <ndbcluster name>-<node-type>-<sfset pod ordinal index>
	tokens := strings.Split(hostname, "-")
	podOrdinalIndex, _ := strconv.ParseInt(tokens[len(tokens)-1], 10, 32)

	// Calculate nodeId
	var nodeIdText string
	if nodeType == mgmapi.NodeTypeAPI {
		// For MySQL Servers, if connection pool is enabled, successive
		// nodeIds are assigned to a single MySQL Server
		ndbConnectionPoolSize, _ := strconv.ParseInt(os.Getenv("NDB_CONNECTION_POOL_SIZE"), 10, 32)
		startNodeId := constants.NdbNodeTypeAPIStartNodeId + int(ndbConnectionPoolSize)*int(podOrdinalIndex)
		endNodeId := startNodeId + int(ndbConnectionPoolSize)
		for ; startNodeId < endNodeId; startNodeId++ {
			nodeIdPool = append(nodeIdPool, startNodeId)
			nodeIdText += fmt.Sprintf("%d,", startNodeId)
		}
		nodeIdText = nodeIdText[:len(nodeIdText)-1]
	} else {
		nodeId = startNodeIdOfSameNodeType + int(podOrdinalIndex)
		nodeIdText = fmt.Sprintf("%d", nodeId)
	}

	// Persist the nodeId into a file to be used by other scripts/commands
	f, err := os.OpenFile(statefulset.NodeIdFilePath,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	failOnError(err, "Failed to create file %q : %s", statefulset.NodeIdFilePath, err)

	_, err = f.WriteString(nodeIdText)
	failOnError(err, "Failed to write nodeId to file : %s", err)

	return nodeId, nodeIdPool, nodeType
}

// waitForNodeIdAvailability waits until the given nodeId can be
// successfully allocated to the MySQL Cluster node that will be run in this pod.
func waitForNodeIdAvailability(nodeId int, nodeType mgmapi.NodeTypeEnum, mgmClient mgmapi.MgmClient) {

	var err error
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

func isSystemRestart(mgmClient mgmapi.MgmClient) bool {
	clusterStatus, err := mgmClient.GetStatus()
	failOnError(err, "Failed to get status from management server : %s", err)

	for _, nodeStatus := range clusterStatus {
		if nodeStatus.IsDataNode() && nodeStatus.IsConnected {
			// Another datanode is already connected to the system => Not a System Restart
			return false
		}
	}

	// No datanodes connected yet
	return true
}

func waitForDataNodeFailureHandlingToComplete(nodeId int, podHostname string) (success bool) {

	// Connect to the MySQL Server - use the StatefulSet's 0th pod
	hostnameTokens := strings.Split(podHostname, "-")
	hostnameTokens[len(hostnameTokens)-1] = "0"
	hostnameTokens[len(hostnameTokens)-2] = constants.NdbNodeTypeMySQLD
	mysqldHost := fmt.Sprintf("%s.%s",
		strings.Join(hostnameTokens, "-"),
		strings.Join(hostnameTokens[:len(hostnameTokens)-1], "-"))

	operatorPassword := os.Getenv("NDB_OPERATOR_PASSWORD")
	db, err := mysqlclient.Connect(mysqldHost, mysqlclient.DbNdbInfo, operatorPassword)
	if err != nil {
		// MySQL Server unavailable
		return false
	}

	// Check if the existing data nodes have completed handling
	// the previous failure of this data node by querying the
	// ndbinfo.restart_info table. When all the STARTED data nodes
	// have completed handling the node failure, the
	// node_restart_status of the current node will be set to
	// "Node failure handling completed".
	var nodeRestartStatus string
	for {
		query := fmt.Sprintf("select node_restart_status from restart_info where node_id='%d'", nodeId)
		err = db.QueryRow(query).Scan(&nodeRestartStatus)
		if err != nil {
			// NdbInfo database unavailable
			log.Printf("Query on ndbinfo.restart_info table failed : %s", err)
			return false
		}

		if nodeRestartStatus == "Node failure handling complete" {
			// Failure handling completed
			return true
		}

		// Sleep and retry
		time.Sleep(500 * time.Millisecond)
	}
}

func main() {

	ctx := context.Background()
	podHostname := os.Getenv("HOSTNAME")

	// Wait for the hostname to be updated in the DNS
	waitForPodDNSUpdate(ctx, podHostname)

	// Persist the nodeId into a file for the other scripts/commands to use
	connectstring := os.Getenv("NDB_CONNECTSTRING")
	nodeId, nodeIdPool, nodeType := writeNodeIDToFile(podHostname, connectstring)

	if nodeType == mgmapi.NodeTypeMGM {
		log.Println("Pod initializer succeeded.")
		return
	}

	// Connect to the Management Server
	var err error
	var mgmClient mgmapi.MgmClient
	mgmClient, err = mgmapi.NewMgmClient(connectstring)
	failOnError(err, "Failed to connect to management server : %s", err)
	defer mgmClient.Disconnect()

	if nodeType == mgmapi.NodeTypeNDB && !isSystemRestart(mgmClient) {
		// Wait for the existing data nodes to finish
		// handling previous data node failure
		log.Println("Waiting for other data nodes to complete Node failure handling...")
		if !waitForDataNodeFailureHandlingToComplete(nodeId, podHostname) {
			// The method failed as either server or the ndbinfo database is not available.
			// Fallback to waitForNodeIdAvailability logic. This is used only as a fallback
			// as the nodeId is freed early in the node failure handling process and due
			// to that waitForNodeIdAvailability is less reliable than checking ndbinfo
			// database.
			log.Println("Falling back to checking if nodeId is available..")
			waitForNodeIdAvailability(nodeId, nodeType, mgmClient)
		}
		log.Println("Done")
	}

	if nodeType == mgmapi.NodeTypeAPI {
		// Wait for all the nodeIds to become available for MySQL nodes
		for _, id := range nodeIdPool {
			waitForNodeIdAvailability(id, nodeType, mgmClient)
		}
	}
}
