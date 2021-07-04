// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import (
	"encoding/json"
	"fmt"
	"testing"
)

func nodeTypeFromNodeId(mgmNodeCount, dataNodeCount, nodeId int) NodeTypeEnum {
	if nodeId <= mgmNodeCount {
		return NodeTypeMGM
	}
	if nodeId <= mgmNodeCount+dataNodeCount {
		return NodeTypeNDB
	}
	// Any node after this point is an API node
	return NodeTypeAPI
}

func Test_ensureNodeAndsetNodeTypeFromTLA(t *testing.T) {

	cs := NewClusterStatus(8)

	cs.ensureNode(1).setNodeTypeFromTLA("MGM")
	cs.ensureNode(2).setNodeTypeFromTLA("MGM")
	cs.ensureNode(3).setNodeTypeFromTLA("NDB")
	cs.ensureNode(4).setNodeTypeFromTLA("NDB")
	cs.ensureNode(5).setNodeTypeFromTLA("NDB")
	cs.ensureNode(6).setNodeTypeFromTLA("API")
	cs.ensureNode(7).setNodeTypeFromTLA("API")
	cs.ensureNode(8).setNodeTypeFromTLA("API")

	dnCnt := 0
	mgmCnt := 0
	apiCnt := 0
	for _, ns := range *cs {
		if ns.IsAPINode() {
			apiCnt++
		}
		if ns.IsDataNode() {
			dnCnt++
		}
		if ns.IsMgmNode() {
			mgmCnt++
		}
	}

	if dnCnt != 3 && apiCnt != 3 && mgmCnt != 2 {
		t.Errorf("Wrong node type count")
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
			NodeId:          nodeID,
			NodeType:        nodeTypeFromNodeId(2, 6, nodeID),
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

	cnt, scale := cs.NumberNodegroupsFullyUp(2)
	if cnt != 2 {
		t.Errorf("Cluster is not degraded but reported wrong node group count %d.", cnt)
	}
	if scale != 1 {
		t.Errorf("Cluster has one scaling node node up but reported wrong node group count %d.", scale)
	}

	if ns, ok := (*cs)[9]; ok {
		(*ns).IsConnected = false
	} else {
		t.Errorf("Defined node id 9 not found.")
		return
	}
	cnt, scale = cs.NumberNodegroupsFullyUp(2)
	if cnt != 2 {
		t.Errorf("Cluster is not degraded with an API node down but reported wrong node group count %d.", cnt)
	}
	if scale != 1 {
		t.Errorf("Cluster is should have same number of scaling nodes with an API node down but reported %d.", scale)
	}

	if ns, ok := (*cs)[3]; ok {
		(*ns).IsConnected = false
	} else {
		t.Errorf("Defined node id 3 not found.")
		return
	}

	cnt, scale = cs.NumberNodegroupsFullyUp(2)
	if cnt != 1 {
		t.Errorf("Cluster is degraded with a data node down but reported wrong node group count %d.", cnt)
	}
	if scale != 1 {
		t.Errorf("Cluster is degraded with a data node down but reported wrong scale node count %d.", scale)
	}

	// Start second scale node
	ns, _ := (*cs)[8]
	(*ns).IsConnected = true
	(*ns).NodeGroup = -256

	cnt, scale = cs.NumberNodegroupsFullyUp(2)
	if cnt != 1 {
		t.Errorf("Cluster is degraded with a data node down but reported wrong node group count %d.", cnt)
	}
	if scale != 2 {
		t.Errorf("Cluster is degraded with a data node down but reported wrong scale node count %d.", scale)
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

	cnt, scale = cs.NumberNodegroupsFullyUp(2)
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
