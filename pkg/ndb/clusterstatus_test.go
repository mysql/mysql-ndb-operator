// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndb

import (
	"encoding/json"
	"fmt"
	"testing"
)

func nodeTypeFromNodeId(mgmNodeCount, dataNodeCount, apiNodeCount, nodeId int) int {

	if nodeId <= mgmNodeCount {
		return MgmNodeTypeID
	}
	if nodeId <= mgmNodeCount+dataNodeCount {
		return DataNodeTypeID
	}
	return APINodeTypeID
}

func Test_AddNodesByTLA(t *testing.T) {

	cs := NewClusterStatus(8)

	cs.SetNodeTypeFromTLA(1, "MGM")
	cs.SetNodeTypeFromTLA(2, "MGM")
	cs.SetNodeTypeFromTLA(3, "NDB")
	cs.SetNodeTypeFromTLA(4, "NDB")
	cs.SetNodeTypeFromTLA(5, "NDB")
	cs.SetNodeTypeFromTLA(6, "API")
	cs.SetNodeTypeFromTLA(7, "API")
	cs.SetNodeTypeFromTLA(8, "API")

	dnCnt := 0
	mgmCnt := 0
	apiCnt := 0
	for _, ns := range *cs {
		if ns.isAPINode() {
			apiCnt++
		}
		if ns.isDataNode() {
			dnCnt++
		}
		if ns.isMgmNode() {
			mgmCnt++
		}
	}

	if dnCnt != 3 && apiCnt != 3 && mgmCnt != 2 {
		t.Errorf("Wrong node type count")
	}

	err := cs.SetNodeTypeFromTLA(2, "MGM__")

	if err == nil {
		t.Errorf("Wrong node type string doesn't produce error")
	}
}

func Test_ClusterIsDegraded(t *testing.T) {

	cs := NewClusterStatus(10)

	// (!) start at 1
	for nodeID := 1; nodeID <= 10; nodeID++ {

		nodeGroup := -1
		connected := true
		if nodeID > 2 && nodeID <= 6 {
			nodeGroup = (nodeID - 1 - 2) / 2
		}
		if nodeID == 7 {
			// mark data node like its a started node in a new group
			nodeGroup = -256
		}
		if nodeID == 8 {
			// mark data node like its an unstarted node in a new group
			connected = false
			nodeGroup = -1
		}

		ns := &NodeStatus{
			NodeID:          nodeID,
			NodeType:        nodeTypeFromNodeId(2, 6, 2, nodeID),
			SoftwareVersion: "8.0.22",
			IsConnected:     connected,
			NodeGroup:       nodeGroup,
		}
		(*cs)[nodeID] = ns
	}

	for nodeID := 1; nodeID <= 10; nodeID++ {
		s, _ := json.Marshal((*cs)[nodeID])
		fmt.Printf("%d - %s\n", nodeID, s)
	}

	if !cs.IsClusterDegraded() {
		t.Errorf("Cluster should be reported degraded by simple IsDegraded function.")
	}

	cnt := cs.NumberNodegroupsFullyUp(2)
	if cnt != 2 {
		t.Errorf("Cluster is not degraded but reported wrong node group count %d.", cnt)
	}

	if ns, ok := (*cs)[9]; ok {
		(*ns).IsConnected = false
	} else {
		t.Errorf("Defined node id 7 not found.")
		return
	}
	cnt = cs.NumberNodegroupsFullyUp(2)
	if cnt != 2 {
		t.Errorf("Cluster is not degraded with an API node down but reported wrong node group count %d.", cnt)
	}

	if ns, ok := (*cs)[3]; ok {
		(*ns).IsConnected = false
	} else {
		t.Errorf("Defined node id 3 not found.")
		return
	}

	cnt = cs.NumberNodegroupsFullyUp(2)
	if cnt != 1 {
		t.Errorf("Cluster is degraded with a data node down but reported wrong node group count %d.", cnt)
	}

	// "Start" cluster

	for nodeID, ns := range *cs {
		nodeGroup := -1
		if nodeID >= 2 && nodeID <= 8 {
			nodeGroup = ((nodeID - 1 - 2) / 2)
		}

		(*ns).IsConnected = true
		(*ns).NodeGroup = nodeGroup
	}

	fmt.Println()
	for nodeID := 1; nodeID <= 10; nodeID++ {
		s, _ := json.Marshal((*cs)[nodeID])
		fmt.Printf("%d - %s\n", nodeID, s)
	}

	if cs.IsClusterDegraded() {
		t.Errorf("Cluster is not degraded but reported degraded.")
	}

	cnt = cs.NumberNodegroupsFullyUp(2)
	if cnt != 3 {
		t.Errorf("Cluster is degraded with a data node down but reported wrong node group count %d.", cnt)
	}

	if ns, ok := (*cs)[3]; ok {
		(*ns).IsConnected = false
	} else {
		t.Errorf("Defined node id 3 not found.")
		return
	}

	if !cs.IsClusterDegraded() {
		t.Errorf("Cluster is not degraded if 1 API Node is down but reported degraded.")
	}
}
