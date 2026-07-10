// Copyright (c) 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/mysql/ndb-operator/pkg/mgmapi"
)

type nodeStatus struct {
	NodeID          int    `json:"nodeId"`
	NodeType        string `json:"nodeType"`
	IsConnected     bool   `json:"isConnected"`
	NodeGroup       int    `json:"nodeGroup"`
	SoftwareVersion string `json:"softwareVersion"`
}

type valueResult struct {
	Value interface{} `json:"value"`
}

func nodeTypeString(nodeType mgmapi.NodeTypeEnum) string {
	switch nodeType {
	case mgmapi.NodeTypeNDB:
		return "NDB"
	case mgmapi.NodeTypeAPI:
		return "API"
	case mgmapi.NodeTypeMGM:
		return "MGM"
	default:
		return fmt.Sprintf("UNKNOWN-%d", nodeType)
	}
}

func printJSON(value interface{}) {
	if err := json.NewEncoder(os.Stdout).Encode(value); err != nil {
		log.Fatalf("failed to encode output: %v", err)
	}
}

// verifyConnectstringReachable gives deny-path tests a bounded TCP failure
// before constructing the management API client.
func verifyConnectstringReachable(connectstring string, timeout time.Duration) error {
	var lastErr error
	for _, host := range strings.Split(connectstring, ",") {
		if strings.HasPrefix(host, "nodeid=") {
			continue
		}

		conn, err := net.DialTimeout("tcp", host, timeout)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
	}

	if lastErr == nil {
		return fmt.Errorf("connectstring %q does not contain a host", connectstring)
	}
	return lastErr
}

func main() {
	connectstring := flag.String("connectstring", "", "NDB management server connectstring")
	connectTimeout := flag.Duration("connect-timeout", 5*time.Second, "TCP connect timeout")
	operation := flag.String("operation", "status", "operation to run")
	nodeID := flag.Int("node-id", 0, "NDB node ID")
	flag.Parse()

	if *connectstring == "" {
		log.Fatal("connectstring is required")
	}

	if err := verifyConnectstringReachable(*connectstring, *connectTimeout); err != nil {
		log.Fatalf("management server is not reachable: %v", err)
	}

	client, err := mgmapi.NewMgmClient(*connectstring)
	if err != nil {
		log.Fatalf("failed to connect to management server: %v", err)
	}
	defer client.Disconnect()

	switch *operation {
	case "status":
		status, err := client.GetStatus()
		if err != nil {
			log.Fatalf("failed to get cluster status: %v", err)
		}
		nodes := make([]nodeStatus, 0, len(status))
		for _, node := range status {
			nodes = append(nodes, nodeStatus{
				NodeID:          node.NodeId,
				NodeType:        nodeTypeString(node.NodeType),
				IsConnected:     node.IsConnected,
				NodeGroup:       node.NodeGroup,
				SoftwareVersion: node.SoftwareVersion,
			})
		}
		printJSON(nodes)

	case "config-version":
		var version uint32
		if *nodeID == 0 {
			version, err = client.GetConfigVersion()
		} else {
			version, err = client.GetConfigVersion(*nodeID)
		}
		if err != nil {
			log.Fatalf("failed to get config version: %v", err)
		}
		printJSON(valueResult{Value: version})

	case "data-memory":
		if *nodeID == 0 {
			log.Fatal("node-id is required for data-memory")
		}
		dataMemory, err := client.GetDataMemory(*nodeID)
		if err != nil {
			log.Fatalf("failed to get data memory: %v", err)
		}
		printJSON(valueResult{Value: dataMemory})

	case "mgmd-arbitration-rank":
		rank, err := client.GetMgmdArbitrationRank()
		if err != nil {
			log.Fatalf("failed to get management arbitration rank: %v", err)
		}
		printJSON(valueResult{Value: rank})

	default:
		log.Fatalf("unsupported operation %q", *operation)
	}
}
