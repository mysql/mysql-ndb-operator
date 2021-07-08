// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import (
	"reflect"
	"testing"
)

func TestClusterStatus_ensureNodeAndSetNodeTypeFromTLA(t *testing.T) {

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
	for _, ns := range cs {
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

func verifyClusterHealth(
	t *testing.T, cs ClusterStatus, csMockUpdate func(cs ClusterStatus), expectHealthyState bool, desc string) {
	t.Helper()

	// copy it to a temp ClusterStatus before making the update
	testCs := make(ClusterStatus, len(cs))
	for k, v := range cs {
		testCs[k] = v
	}

	// Make the update
	if csMockUpdate != nil {
		csMockUpdate(testCs)
	}

	if cs.IsHealthy() != expectHealthyState {
		if expectHealthyState {
			t.Error("Expected the ClusterState to be healthy but it was not. Case :", desc)
		} else {
			t.Error("Expected the ClusterState to be not healthy but it was. Case :", desc)
		}
	}

}

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

func getClusterStatus() ClusterStatus {
	cs := NewClusterStatus(6)
	for nodeId := 1; nodeId <= 6; nodeId++ {

		nodeType := nodeTypeFromNodeId(2, 4, nodeId)
		nodeGroup := 0
		if nodeType == NodeTypeNDB {
			// 3,4 => 0, 5,6 => 1
			switch nodeId {
			case 3, 4:
				nodeGroup = 0
			case 5, 6:
				nodeGroup = 1
			}
		}

		cs[nodeId] = &NodeStatus{
			NodeId:          nodeId,
			NodeType:        nodeTypeFromNodeId(2, 4, nodeId),
			SoftwareVersion: "8.0.22",
			IsConnected:     true,
			NodeGroup:       nodeGroup,
		}
	}

	return cs
}

func TestClusterStatus_IsHealthy(t *testing.T) {

	cs := getClusterStatus()

	for _, tc := range []struct {
		desc               string
		csMockUpdate       func(cs ClusterStatus)
		expectHealthyState bool
	}{
		{
			desc:               "verify original cs is healthy",
			expectHealthyState: true,
		},
		{
			desc: "a node with NG -256 and connected is healthy",
			csMockUpdate: func(cs ClusterStatus) {
				// start one node at 65536
				cs[6].NodeGroup = -256
			},
			expectHealthyState: true,
		},
		{
			desc: "a node with NG 65536 and not connected is healthy",
			csMockUpdate: func(cs ClusterStatus) {
				cs[5].NodeGroup = 65536
				cs[5].IsConnected = false
				cs[6].NodeGroup = 65536
				cs[6].IsConnected = false
			},
			expectHealthyState: true,
		},
		{
			desc: "a disconnected data node is not healthy",
			csMockUpdate: func(cs ClusterStatus) {
				cs[3].IsConnected = false
			},
			expectHealthyState: false,
		},
		{
			desc: "a disconnected mgmd node is not healthy",
			csMockUpdate: func(cs ClusterStatus) {
				cs[1].IsConnected = false
			},
			expectHealthyState: false,
		},
	} {
		verifyClusterHealth(t, cs, tc.csMockUpdate, tc.expectHealthyState, tc.desc)
	}
}

func TestClusterStatus_GetNodesGroupedByNodegroup(t *testing.T) {
	cs := getClusterStatus()
	nodes := cs.GetNodesGroupedByNodegroup()

	// verify the nodes are grouped as expected
	expectedReply := [][]int{{3, 4}, {5, 6}}
	if !reflect.DeepEqual(nodes, expectedReply) {
		t.Errorf("Expected grouping : %#v", expectedReply)
		t.Errorf("Actual grouping : %#v", nodes)
	}
}
