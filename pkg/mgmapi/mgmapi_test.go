// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

const connectstring = "127.0.0.1:1186"

func getConnectionToMgmd(t *testing.T) *mgmClientImpl {
	t.Helper()

	// create a mgm client and connect to mgmd
	mgmClient := &mgmClientImpl{}
	err := mgmClient.connect(connectstring)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			// Management server not running. Skip the test
			t.Skipf("Skipping due to error : %v", err)
		}

		// Connection failing due to some other error
		t.Errorf("Connection failed: %s", err)
		return nil
	}

	return mgmClient
}

// getAnyConnectedNodeId returns id of a connected node of type nodeType
func getAnyConnectedNodeId(t *testing.T, mc MgmClient, nodeType NodeTypeEnum) int {
	t.Helper()
	status, err := mc.GetStatus()
	if err != nil {
		t.Errorf("Failed to get cluster status : %s", err)
		return 0
	}

	for _, node := range *status {
		if node.IsConnected && node.NodeType == nodeType {
			return node.NodeId
		}
	}

	t.Errorf("Failed to get a connected node of type : %s", nodeType.toString())
	return 0
}

func TestMgmClientImpl_connectToNodeId(t *testing.T) {
	// Fetch a connected data node id first
	mci := getConnectionToMgmd(t)
	defer mci.Disconnect()

	desiredNodeId := getAnyConnectedNodeId(t, mci, NodeTypeMGM)
	if desiredNodeId == 0 {
		return
	}

	// close this connection
	mci.Disconnect()

	// Now, create a connection to the desiredNodeId
	err := mci.connectToNodeId(connectstring, desiredNodeId)
	if err != nil {
		t.Errorf("Connection failed: %s", err)
		return
	}
	defer mci.Disconnect()

	nodeid, err := mci.getConnectedMgmdNodeId()
	if err != nil {
		t.Errorf("getConnectedMgmdNodeId failed: %s", err)
		return
	}

	if nodeid != desiredNodeId {
		t.Errorf("Connecting to wanted node id %d failed with wrong or unknown node id: %d", desiredNodeId, nodeid)
	}
}

func TestMgmClientImpl_sendCommand(t *testing.T) {
	mci := getConnectionToMgmd(t)
	defer mci.Disconnect()

	// test a valid command - get session id
	command := "get session id"
	reply, err := mci.sendCommand(command, nil, false)
	if err != nil {
		t.Errorf("%s command failed : %s", command, err)
	} else if !strings.HasPrefix(string(reply), command+" reply") {
		t.Errorf("unexpected reply received for '%s' : \n%s", command, reply)
	}

	// test a valid command with args - get session
	command = "get session"
	reply, err = mci.sendCommand(command, map[string]interface{}{
		"id": 1,
	}, false)
	if err != nil {
		t.Errorf("%s command failed : %s", command, err)
	} else if !strings.HasPrefix(string(reply), command+" reply") {
		t.Errorf("unexpected reply received for '%s' : \n%s", command, reply)
	}
}

// parseReplyAndExpectToPanic calls parseReply on a reply with
// wrong format and verifies that it panics.
func parseReplyAndExpectToPanic(
	t *testing.T, mci *mgmClientImpl, desc, command string, reply []byte,
	expectedDetails []string, expectedError string) {

	// the function is expected to panic
	defer func() {
		err := recover()
		if err != nil && err != expectedError {
			t.Errorf("parseReply with '%s' panicked with unexpected error : %s", desc, err)
		}
	}()

	values, _ := mci.parseReply(command, reply, expectedDetails)
	t.Errorf("expected parseReply with '%s' to panic but succeeded. values : %#v", desc, values)
}

func TestMgmClientImpl_parseReply(t *testing.T) {
	mci := getConnectionToMgmd(t)
	defer mci.Disconnect()

	// default reply that will be tested with
	getSessionReply := []byte("get session reply\nid: 42\nm_stopSelf: always\nm_stop: 0 \n\n")

	// test error scenarios
	invalidTestcases := []*struct {
		desc, command   string
		reply           []byte
		expectedDetails []string
		expectedError   string
	}{
		{
			desc:          "invalid command",
			reply:         []byte("result: Unknown command, 'get time'"),
			expectedError: "sendCommand failed : Unknown command",
		},
		{
			desc:          "invalid argument",
			reply:         []byte("result: Unknown argument, 'operator-id'"),
			expectedError: "sendCommand failed : Unknown argument",
		},
		{
			desc:          "missing mandatory command",
			reply:         []byte("result: Missing arg., '"),
			expectedError: "sendCommand failed : Missing arg",
		},
		{
			desc:            "wrong header",
			command:         "get id",
			expectedDetails: []string{"id reply"},
			expectedError:   "unexpected header in reply",
		},
		{
			desc:            "missing colon",
			command:         "get session",
			reply:           []byte("get session reply\nid 42\n\n"),
			expectedDetails: []string{"get session reply"},
			expectedError:   "missing colon(:) in reply detail",
		},
		{
			desc:            "too much expected details",
			command:         "get session",
			expectedDetails: []string{"get session reply", "id", "m_stopSelf", "m_stop", "m_start", "m_pause"},
			expectedError:   "expected detail not found in reply",
		},
		{
			desc:            "invalid expected details",
			command:         "get session",
			expectedDetails: []string{"get session reply", "id", "m_start"},
			expectedError:   "expected detail not found in reply",
		},
	}

	for _, tc := range invalidTestcases {
		reply := tc.reply
		if reply == nil {
			reply = getSessionReply
		}
		parseReplyAndExpectToPanic(t, mci, tc.desc, tc.command, reply, tc.expectedDetails, tc.expectedError)
	}

	// test successful parsing
	values, err := mci.parseReply(
		"get session", getSessionReply,
		[]string{"get session reply", "id", "m_stopSelf", "m_stop"})
	if err != nil {
		t.Errorf("expected parseReply to succeed but failed. error : %s", err)
	}

	expectedValues := map[string]string{
		"id":         "42",
		"m_stopSelf": "always",
		"m_stop":     "0",
	}

	if !reflect.DeepEqual(values, expectedValues) {
		t.Error("parseReply failed to extract right reply values")
		t.Errorf("  Expected : %#v", expectedValues)
		t.Errorf("  Actual   : %#v", values)
	}
}

// executeCommandAndExpectToFail sends a command with
// invalid format and expects the client to panic
func executeCommandAndExpectToFail(
	t *testing.T, mci *mgmClientImpl, desc, command string,
	args map[string]interface{}, expectedReplyDetails []string, expectedError string) {

	// the function is expected to panic
	defer func() {
		err := recover()
		if err != nil && err != expectedError {
			t.Errorf("executeCommand with '%s' panicked with unexpected error : %s", desc, err)
		}
	}()

	// test the command
	reply, err := mci.executeCommand(command, args, false, expectedReplyDetails)
	if err != nil {
		if err.Error() != expectedError {
			t.Errorf("executeCommand with '%s' failed with unexpected error : %s", desc, err)
		}
	} else {
		t.Errorf("Expected executeCommand with '%s' to panic/fail but succeeded. Reply recieved : %s", desc, reply)
	}
}

func TestMgmClientImpl_executeCommand(t *testing.T) {
	mci := getConnectionToMgmd(t)
	defer mci.Disconnect()

	// test scenarios
	testCases := []*struct {
		desc, command   string
		args            map[string]interface{}
		expectedDetails []string
		// setting expectedError => test case
		// is expected to panic with the error
		expectedError string
		// success cases can optionally set this
		// func to validate the reply returned
		validateReply func(map[string]string) bool
	}{
		// failure cases
		{
			desc:          "invalid command",
			command:       "get time",
			expectedError: "sendCommand failed : Unknown command",
		},
		{
			desc:    "invalid argument name",
			command: "get config",
			args: map[string]interface{}{
				"operator-id": "0.1",
			},
			expectedError: "sendCommand failed : Unknown argument",
		},
		{
			desc:    "missing mandatory command",
			command: "get config",
			args: map[string]interface{}{
				"node": "1",
			},
			expectedError: "sendCommand failed : Missing arg",
		},
		{
			desc:    "invalid argument type",
			command: "get config",
			args: map[string]interface{}{
				"node": "invalid",
			},
			expectedError: "sendCommand failed : Type mismatch",
		},
		{
			desc:            "invalid expected details",
			command:         "get session id",
			expectedDetails: []string{"get session id reply", "id", "m_start"},
			expectedError:   "expected detail not found in reply",
		},
		{
			desc:    "invalid argument",
			command: "get config_v2",
			args: map[string]interface{}{
				"version":   "0",
				"from_node": "1000",
			},
			expectedDetails: []string{"get config reply", "result", "Content-Length"},
			expectedError:   "Nodeid 1000 is greater than max nodeid 255.",
		},
		// success cases
		{
			desc:            "command with nil args",
			command:         "get session id",
			expectedDetails: []string{"get session id reply", "id"},
			validateReply: func(reply map[string]string) bool {
				_, err := strconv.Atoi(reply["id"])
				if err != nil {
					t.Error("failed to parse the returned id")
					return false
				}
				return true
			},
		},
		{
			desc:    "command with args",
			command: "get session",
			args: map[string]interface{}{
				"id": "1",
			},
			expectedDetails: []string{"get session reply", "id", "m_stopSelf", "m_stop"},
			validateReply: func(reply map[string]string) bool {
				_, err := strconv.Atoi(reply["parser_status"])
				if err != nil {
					t.Error("failed to parse the returned parser_status")
					return false
				}
				return true
			},
		},
	}

	for _, tc := range testCases {
		if tc.expectedError == "" {
			reply, err := mci.executeCommand(tc.command, tc.args, false, tc.expectedDetails)
			if err != nil {
				t.Errorf("executeCommand(desc : %s) failed : %s", tc.desc, err)
			}
			if tc.validateReply != nil {
				tc.validateReply(reply)
			}
		} else {
			executeCommandAndExpectToFail(t, mci, tc.desc, tc.command, tc.args, tc.expectedDetails, tc.expectedError)
		}
	}
}

func TestMgmClientImpl_getConnectedMgmdNodeId(t *testing.T) {
	mci := getConnectionToMgmd(t)
	defer mci.Disconnect()

	nodeid, err := mci.getConnectedMgmdNodeId()
	if err != nil {
		t.Fatalf("get connected mgmd node id failed : %s", err)
	}

	t.Logf("Connected mgmd node id : %d", nodeid)
}

func TestMgmClientImpl_GetStatus(t *testing.T) {
	mci := getConnectionToMgmd(t)
	defer mci.Disconnect()

	clusterStatus, err := mci.GetStatus()
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

func TestMgmClientImpl_StopNodes(t *testing.T) {
	mci := getConnectionToMgmd(t)
	defer mci.Disconnect()

	dataNodeId := getAnyConnectedNodeId(t, mci, NodeTypeNDB)
	if dataNodeId == 0 {
		return
	}

	if err := mci.StopNodes([]int{dataNodeId}); err != nil {
		t.Errorf("stop failed : %s\n", err)
		return
	}
}

func TestMgmClientImpl_getConfig(t *testing.T) {
	mci := getConnectionToMgmd(t)
	defer mci.Disconnect()

	_, err := mci.getConfig(cfgSectionTypeNDB, dbCfgDataMemory)
	if err != nil {
		t.Errorf("getting config failed : %s", err)
		return
	}

	dataNodeId := getAnyConnectedNodeId(t, mci, NodeTypeNDB)
	if dataNodeId == 0 {
		return
	}

	_, err = mci.getConfigFromNode(dataNodeId, cfgSectionTypeNDB, dbCfgTransactionMemory)
	if err != nil {
		t.Errorf("getting config failed : %s", err)
		return
	}
}
func TestMgmClientImpl_GetConfigVersionFromNode(t *testing.T) {
	mci := getConnectionToMgmd(t)
	defer mci.Disconnect()

	dataNodeId := getAnyConnectedNodeId(t, mci, NodeTypeNDB)
	if dataNodeId == 0 {
		return
	}

	version, err := mci.GetConfigVersionFromNode(dataNodeId)
	if err != nil {
		t.Errorf("GetConfigVersionFromNode nodeId:%d failed : %s", dataNodeId, err)
	} else {
		t.Logf("Extracted config version: %d", version)
	}

	// Test the default version
	version, err = mci.GetConfigVersion()
	if err != nil {
		t.Errorf("GetConfigVersionFromNode nodeId:%d failed : %s", dataNodeId, err)
	} else {
		t.Logf("Extracted config version: %d", version)
	}
}
