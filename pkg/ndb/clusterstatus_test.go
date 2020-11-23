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

	cs := NewClusterStatus(8)

	// (!) start at 1
	for nodeID := 1; nodeID <= 8; nodeID++ {
		ns := &NodeStatus{
			NodeID:          nodeID,
			NodeType:        nodeTypeFromNodeId(2, 4, 2, nodeID),
			SoftwareVersion: "8.0.22",
			IsConnected:     true,
		}
		(*cs)[nodeID] = ns
	}

	for nodeID, ns := range *cs {
		s, _ := json.Marshal(ns)
		fmt.Printf("%d - %s\n", nodeID, s)
	}

	if cs.IsClusterDegraded() {
		t.Errorf("Cluster is not degraded but reported degraded.")
	}

	if ns, ok := (*cs)[7]; ok {
		(*ns).IsConnected = false
	} else {
		t.Errorf("Defined node id 1 not found.")
		return
	}
	if cs.IsClusterDegraded() {
		t.Errorf("Cluster is not degraded if 1 API Node is down but reported degraded.")
	}

	if ns, ok := (*cs)[1]; ok {
		(*ns).IsConnected = false
	} else {
		t.Errorf("Defined node id 1 not found.")
		return
	}

	if !cs.IsClusterDegraded() {
		t.Errorf("Cluster is degraded but reported not degraded.")
	}

}
