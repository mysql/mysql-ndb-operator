// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndb

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const connectstring = "127.0.0.1:1186"

func getConnectionToMgmd(t *testing.T) *Mgmclient {
	t.Helper()

	// create a mgm client and connect to mgmd
	api := &Mgmclient{}

	err := api.Connect(connectstring)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			// Management server not running. Skip the test
			t.Skipf("Skipping due to error : %v", err)
		}

		// Connection failing due to some other error
		t.Errorf("Connection failed: %s", err)
		return nil
	}

	return api
}

func getConnectedNodeId(t *testing.T, api *Mgmclient, nodeType int) int {
	t.Helper()
	status, err := api.GetStatus()
	if err != nil {
		t.Errorf("Failed to get cluster status : %s", err)
		return 0
	}

	for _, node := range *status {
		if node.IsConnected && node.NodeType == nodeType {
			return node.NodeID
		}
	}

	t.Errorf("Failed to get a connected data node ID : %s", err)
	return 0
}

func TestGetStatus(t *testing.T) {
	api := getConnectionToMgmd(t)
	if api == nil {
		return
	}
	defer api.Disconnect()

	clusterStatus, err := api.GetStatus()
	if err != nil {
		t.Errorf("get status failed: %s", err)
		return
	}

	for s, v := range *clusterStatus {
		vs, _ := json.MarshalIndent(v, "", " ")
		fmt.Printf("[%d] %s ", s, vs)
	}

	ok := clusterStatus.IsClusterDegraded()
	if ok {
		t.Errorf("Cluster is in degraded state\n")
	}

}

func TestGetOwnNodeId(t *testing.T) {
	api := getConnectionToMgmd(t)
	if api == nil {
		return
	}
	defer api.Disconnect()

	nodeid, err := api.GetOwnNodeId()
	if err != nil {
		t.Errorf("get status failed: %s", err)
		return
	}

	fmt.Printf("Own nodeid: %d\n", nodeid)
}

func TestStopNodes(t *testing.T) {
	api := getConnectionToMgmd(t)
	if api == nil {
		return
	}
	defer api.Disconnect()

	dataNodeId := getConnectedNodeId(t, api, DataNodeTypeID)
	if dataNodeId == 0 {
		return
	}

	if disconnect, err := api.StopNodes(&[]int{dataNodeId}); err != nil {
		t.Errorf("stop failed : %s\n", err)
		return
	} else if disconnect {
		t.Errorf("Data node disconnected.")
		return
	}
}

func TestGetConfig(t *testing.T) {
	api := getConnectionToMgmd(t)
	if api == nil {
		return
	}
	defer api.Disconnect()

	_, err := api.GetConfig()
	if err != nil {
		t.Errorf("getting config failed : %s", err)
		return
	}

	dataNodeId := getConnectedNodeId(t, api, DataNodeTypeID)
	if dataNodeId == 0 {
		return
	}

	_, err = api.GetConfigFromNode(dataNodeId)
	if err != nil {
		t.Errorf("getting config failed : %s", err)
		return
	}
}
func Test_GetConfigVersionFromNode(t *testing.T) {
	api := getConnectionToMgmd(t)
	if api == nil {
		return
	}
	defer api.Disconnect()

	dataNodeId := getConnectedNodeId(t, api, DataNodeTypeID)
	if dataNodeId == 0 {
		return
	}

	version := api.GetConfigVersionFromNode(dataNodeId)
	fmt.Printf("getting config version: %d\n", version)
}

func TestShowConfig(t *testing.T) {
	api := getConnectionToMgmd(t)
	if api == nil {
		return
	}
	defer api.Disconnect()

	err := api.showConfig()
	if err != nil {
		t.Errorf("getting config failed : %s", err)
		return
	}
}

func TestShowVariables(t *testing.T) {
	api := getConnectionToMgmd(t)
	if api == nil {
		return
	}
	defer api.Disconnect()

	nodeid, err := api.GetOwnNodeId()
	if err != nil {
		t.Errorf("show variables failed: %s", err)
		return
	}

	if nodeid == 0 {
		t.Errorf("show variables failed with wrong or unknown node id: %d", nodeid)
	}
}

func TestConnectWantedNodeId(t *testing.T) {
	// Fetch a connected data node id first
	api := getConnectionToMgmd(t)
	if api == nil {
		return
	}

	wantedNodeId := getConnectedNodeId(t, api, MgmNodeTypeID)
	if wantedNodeId == 0 {
		return
	}

	// close this connection
	api.Disconnect()

	// Now, connect to the wantedNodeId
	err := api.ConnectToNodeId(connectstring, wantedNodeId)
	if err != nil {
		t.Errorf("Connection failed: %s", err)
		return
	}
	defer api.Disconnect()

	nodeid, err := api.GetOwnNodeId()
	if err != nil {
		t.Errorf("show variables failed: %s", err)
		return
	}

	if nodeid != wantedNodeId {
		t.Errorf("Connecting to wanted node id %d failed with wrong or unknown node id: %d", wantedNodeId, nodeid)
	}
}
