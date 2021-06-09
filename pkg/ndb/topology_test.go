// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndb

import (
	"encoding/json"
	"fmt"
	"testing"
)

func Test_ClusterTopologyByReplica(t *testing.T) {
	cs := NewClusterStatus(8)

	ng := 0
	ngCnt := 0
	// (!) start at 1
	for nodeID := 1; nodeID <= 8; nodeID++ {
		ns := &NodeStatus{
			NodeId:          nodeID,
			NodeType:        nodeTypeFromNodeId(2, 4, nodeID),
			SoftwareVersion: "8.0.22",
			IsConnected:     true,
		}

		if ns.IsDataNode() {
			ns.NodeGroup = ng
			ngCnt++
			if ngCnt > 1 {
				ngCnt = 0
				ng++
			}
		}

		(*cs)[nodeID] = ns
	}

	for nodeID, ns := range *cs {
		s, _ := json.Marshal(ns)
		fmt.Printf("%d - %s\n", nodeID, s)
	}

	ct := CreateClusterTopologyByReplicaFromClusterStatus(cs)

	if ct.GetNumberOfReplicas() != 2 {
		t.Errorf("Wrong number of replicas")
	}
	if ct.GetNumberOfNodeGroups() != 2 {
		t.Errorf("Wrong number of node groups")
	}

	for replicaID := 0; replicaID < 2; replicaID++ {
		nodeIDs := ct.GetNodeIDsFromReplica(replicaID)
		s, _ := json.Marshal(nodeIDs)
		fmt.Printf("replica [%d]: %s\n", replicaID, s)
	}

}
