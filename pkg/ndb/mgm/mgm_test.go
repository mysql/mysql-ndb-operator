// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgm

import (
	"fmt"
	"testing"
)

const connectstring = "127.0.0.1:1186"

func TestGetMgmNodeId(t *testing.T) {

	api := new(Client)

	err := api.Connect(connectstring)
	if err != nil {
		t.Errorf("Connection failed: %s", err)
		return
	}
	defer api.Disconnect()

	nodeid, err := api.GetMgmNodeId()
	if err != nil {
		t.Errorf("get status failed: %s", err)
		return
	}

	fmt.Printf("Own nodeid: %d\n", nodeid)
}

// This test depends on Nodes 3 and 4 being data nodes
// Fixme: first fetch the config, then stop a known data node from the config
func TestStopNodes(t *testing.T) {
	api := new(Client)

	err := api.Connect(connectstring)
	if err != nil {
		t.Errorf("Connection failed: %s\n", err)
		return
	}
	defer api.Disconnect()

	nodeIds := []int{3, 4}
	disconnect, err := api.StopNodes(&nodeIds)
	if err != nil {
		t.Errorf("stop failed : %s\n", err)
		return
	}

	if disconnect {
		fmt.Println("Disconnect")
	}

}

func TestGetConfig(t *testing.T) {
	api := new(Client)

	err := api.Connect(connectstring)
	if err != nil {
		t.Errorf("Connection failed: %s", err)
		return
	}
	defer api.Disconnect()

	verId, err := api.GetVersion(nil)
	if err != nil {
		t.Errorf("getting version")
	}

	_, err = api.GetConfig(0, verId)
	if err != nil {
		t.Errorf("getting config failed : %s", err)
		return
	}

	// _, err = api.GetConfig(3, verId)
	// if err != nil {
	//   t.Errorf("getting config failed : %s", err)
	//   return
	// }
}

func Test_GetConfigVersion(t *testing.T) {
	api := new(Client)

	err := api.Connect(connectstring)
	if err != nil {
		t.Errorf("Connection failed: %s", err)
		return
	}
	defer api.Disconnect()

	version, err := api.GetConfigVersion()
	fmt.Printf("getting config version: %d\n", version)
}

func TestShowVariables(t *testing.T) {
	api := new(Client)

	err := api.Connect(connectstring)
	if err != nil {
		t.Errorf("Connection failed: %s", err)
		return
	}
	defer api.Disconnect()

	nodeid, err := api.GetMgmNodeId()
	if err != nil {
		t.Errorf("show variables failed: %s", err)
		return
	}

	if nodeid == 0 {
		t.Errorf("show variables failed with wrong or unknown node id: %d", nodeid)
	}
}

// FIXME This test dependes on a particular config
// Fetch a configuration first, then compare the results to it
func TestConnectWantedNodeId(t *testing.T) {
	api := new(Client)

	wantedNodeId := 2
	err := api.ConnectToNodeId(connectstring, wantedNodeId)
	if err != nil {
		t.Error(err)
	}
	defer api.Disconnect()

	nodeid, err := api.GetMgmNodeId()
	if err != nil {
		t.Errorf("show variables failed: %s", err)
		return
	}

	if nodeid != wantedNodeId {
		t.Errorf("Connecting to wanted node id %d failed with wrong or unknown node id: %d", wantedNodeId, nodeid)
	}
}
