// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndb

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestGetStatus(t *testing.T) {
	api := &Mgmclient{}

	err := api.Connect()
	if err != nil {
		t.Errorf("Connection failed: %s", err)
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
}

func TestGetOwnNodeId(t *testing.T) {

	api := &Mgmclient{}

	err := api.Connect()
	if err != nil {
		t.Errorf("Connection failed: %s", err)
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
	api := &Mgmclient{}

	err := api.Connect()
	if err != nil {
		t.Errorf("Connection failed: %s", err)
		return
	}
	defer api.Disconnect()

	nodeIds := []int{2}
	disconnect, err := api.StopNodes(&nodeIds)
	if err != nil {
		t.Errorf("stop failed : %s", err)
		return
	}

	if disconnect {
		fmt.Println("Disconnect")
	}

}

func TestGetConfig(t *testing.T) {
	api := &Mgmclient{}

	err := api.Connect()
	if err != nil {
		t.Errorf("Connection failed: %s", err)
		return
	}
	defer api.Disconnect()

	_, err = api.GetConfig()
	if err != nil {
		t.Errorf("getting config failed : %s", err)
		return
	}

	_, err = api.GetConfigFromNode(2)
	if err != nil {
		t.Errorf("getting config failed : %s", err)
		return
	}
}

func TestShowConfig(t *testing.T) {
	api := &Mgmclient{}

	err := api.Connect()
	if err != nil {
		t.Errorf("Connection failed: %s", err)
		return
	}
	defer api.Disconnect()

	err = api.showConfig()
	if err != nil {
		t.Errorf("getting config failed : %s", err)
		return
	}
}

func TestShowVariables(t *testing.T) {
	api := &Mgmclient{}

	err := api.Connect()
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

	if nodeid == 0 {
		t.Errorf("show variables failed with wrong or unknown node id: %d", nodeid)
	}
}

func TestConnectWantedNodeId(t *testing.T) {
	api := &Mgmclient{}

	wantedNodeId := 2
	err := api.ConnectToNodeId(wantedNodeId)
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
