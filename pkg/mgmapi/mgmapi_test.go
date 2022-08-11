// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import (
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

// expectReturnValue checks that the returned value is same as the expected value
func expectReturnValue(t *testing.T, err error, returnValue uint64, expectedValue uint64) {
	t.Helper()

	if err != nil {
		t.Errorf("getting value for config parameter from mgm server failed : %s", err)
		return
	}

	if returnValue != expectedValue {
		t.Errorf("default value for the config parameter differ from the returned value. Returned value:%d expected value:%d", returnValue, expectedValue)
		return
	}
}

// getAnyConnectedNodeId returns id of a connected node of type nodeType
func getAnyConnectedNodeId(t *testing.T, mc MgmClient, nodeType NodeTypeEnum) int {
	t.Helper()
	status, err := mc.GetStatus()
	if err != nil {
		t.Errorf("Failed to get cluster status : %s", err)
		return 0
	}

	for _, node := range status {
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

// parseReplyAndExpectToFail calls executeCommand to parse a preset reply
// (using fake mgm server) and expects the command to throw or fail with an error.
func parseReplyAndExpectToFail(
	t *testing.T, desc string, reply []byte,
	expectedDetails []string, expectedError string) {

	// the function is expected to panic in debug builds
	defer func() {
		err := recover()
		if err != nil && !strings.Contains(err.(string), expectedError) {
			t.Errorf("parseReply with '%s' panicked with unexpected error : %s", desc, err)
		}
	}()

	// Run fake mgmd server
	mgmServer, mci := newFakeMgmServerAndClient(t)
	defer mci.Disconnect()
	defer mgmServer.disconnect()
	mgmServer.run(reply)

	// Run execute command, read back and parse pre-set replies and check if any error is thrown.
	// Use an empty command and nil args as the preset replies are not dependent on those.
	values, err := mci.executeCommand("", nil, false, expectedDetails)
	if err != nil {
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("parseReply with '%s' failed with unexpected error : %s", desc, err)
		}
	} else {
		t.Errorf("expected parseReply with '%s' to fail but succeeded. values : %#v", desc, values)
	}
}

// TestMgmClientImpl_executeCommand_replyParser tests the parser inside executeCommand method
func TestMgmClientImpl_executeCommand_replyParser(t *testing.T) {

	// default reply that will be tested with
	getSessionReply := []byte("get session reply\nid: 42\nm_stopSelf: always\nm_stop: 0 ")

	// test error scenarios
	invalidTestcases := []*struct {
		desc, command   string
		reply           []byte
		expectedDetails []string
		expectedError   string
	}{
		{
			desc:            "invalid command",
			reply:           []byte("result: Unknown command, 'get time'"),
			expectedDetails: []string{"get time reply"},
			expectedError:   "executeCommand failed : Unknown command",
		},
		{
			desc:            "invalid argument",
			reply:           []byte("result: Unknown argument, 'operator-id'"),
			expectedDetails: []string{"get time reply"},
			expectedError:   "executeCommand failed : Unknown argument",
		},
		{
			desc:            "missing mandatory command",
			reply:           []byte("result: Missing arg., '"),
			expectedDetails: []string{"get time reply"},
			expectedError:   "executeCommand failed : Missing arg",
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
			reply:           []byte("get session reply\nid 42"),
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
		parseReplyAndExpectToFail(t, tc.desc, reply, tc.expectedDetails, tc.expectedError)
	}

	// test successful parsing
	mgmServer, mci := newFakeMgmServerAndClient(t)
	defer mci.Disconnect()
	defer mgmServer.disconnect()
	mgmServer.run(getSessionReply)
	values, err := mci.executeCommand("", nil, false,
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
		if err != nil && !strings.Contains(err.(string), expectedError) {
			t.Errorf("executeCommand with '%s' panicked with unexpected error : %s", desc, err)
		}
	}()

	// test the command
	reply, err := mci.executeCommand(command, args, false, expectedReplyDetails)
	if err != nil {
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("executeCommand with '%s' failed with unexpected error : %s", desc, err)
		}
	} else {
		t.Errorf("Expected executeCommand with '%s' to panic/fail but succeeded. Reply recieved : %s", desc, reply)
	}
}

func TestMgmClientImpl_executeCommand(t *testing.T) {
	// Run test on actual Mgmd Server
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
			desc:            "invalid command",
			command:         "get time",
			expectedDetails: []string{"get time reply"},
			expectedError:   "executeCommand failed : Unknown command, 'get time'",
		},
		{
			desc:    "invalid argument name",
			command: "get config",
			args: map[string]interface{}{
				"operator-id": "0.1",
			},
			expectedDetails: []string{"get time reply"},
			expectedError:   "executeCommand failed : Unknown argument, 'operator-id'",
		},
		{
			desc:    "missing mandatory command",
			command: "get config",
			args: map[string]interface{}{
				"node": "1",
			},
			expectedDetails: []string{"get time reply"},
			expectedError:   "executeCommand failed : Missing arg",
		},
		{
			desc:    "invalid argument type",
			command: "get config",
			args: map[string]interface{}{
				"node": "invalid",
			},
			expectedDetails: []string{"get time reply"},
			expectedError:   "executeCommand failed : Type mismatch",
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

	for s, v := range clusterStatus {
		t.Logf("[%d] %#v", s, v)
	}
}

func TestMgmClientImpl_StopNodes(t *testing.T) {
	mci := getConnectionToMgmd(t)
	defer mci.Disconnect()

	dataNodeId := getAnyConnectedNodeId(t, mci, NodeTypeNDB)
	if dataNodeId == 0 {
		return
	}

	err := mci.StopNodes([]int{dataNodeId})
	// Allow 'Node shutdown would cause system crash' error
	if err != nil &&
		!strings.Contains(err.Error(), "Node shutdown would cause system crash") {
		t.Errorf("stop failed : %s\n", err)
		return
	}
}

func TestMgmClientImpl_getConfig(t *testing.T) {
	mci := getConnectionToMgmd(t)
	defer mci.Disconnect()

	dataNodeId := getAnyConnectedNodeId(t, mci, NodeTypeNDB)
	if dataNodeId == 0 {
		return
	}

	_, err := mci.getConfig(dataNodeId, cfgSectionTypeNDB, dbCfgTransactionMemory, false)
	if err != nil {
		t.Errorf("getting config failed : %s", err)
		return
	}
}

func TestMgmClientImpl_getDefaultConfig(t *testing.T) {
	mci := getConnectionToMgmd(t)
	defer mci.Disconnect()

	dataNodeId := getAnyConnectedNodeId(t, mci, NodeTypeNDB)
	if dataNodeId == 0 {
		return
	}

	returnValue, err := mci.GetDataMemory(dataNodeId)
	expectReturnValue(t, err, returnValue, 102760448)

	returnValue32, err := mci.GetMaxNoOfTables(dataNodeId)
	expectReturnValue(t, err, uint64(returnValue32), 128)

	returnValue32, err = mci.GetMaxNoOfAttributes(dataNodeId)
	expectReturnValue(t, err, uint64(returnValue32), 1000)

	returnValue32, err = mci.GetMaxNoOfOrderedIndexes(dataNodeId)
	expectReturnValue(t, err, uint64(returnValue32), 128)

	returnValue32, err = mci.GetMaxNoOfUniqueHashIndexes(dataNodeId)
	expectReturnValue(t, err, uint64(returnValue32), 64)

	returnValue32, err = mci.GetMaxNoOfConcurrentOperations(dataNodeId)
	expectReturnValue(t, err, uint64(returnValue32), 32768)

	returnValue32, err = mci.GetTransactionBufferMemory(dataNodeId)
	expectReturnValue(t, err, uint64(returnValue32), 1048576)

	returnValue, err = mci.GetIndexMemory(dataNodeId)
	expectReturnValue(t, err, returnValue, 0)

	returnValue32, err = mci.GetRedoBuffer(dataNodeId)
	expectReturnValue(t, err, uint64(returnValue32), 33554432)

	returnValue32, err = mci.GetLongMessageBuffer(dataNodeId)
	expectReturnValue(t, err, uint64(returnValue32), 67108864)

	returnValue, err = mci.GetDiskPageBufferMemory(dataNodeId)
	expectReturnValue(t, err, returnValue, 67108864)

	returnValue, err = mci.GetSharedGlobalMemory(dataNodeId)
	expectReturnValue(t, err, returnValue, 134217728)

	returnValue, err = mci.GetTransactionMemory(dataNodeId)
	expectReturnValue(t, err, returnValue, 0)

	returnValue32, err = mci.GetNoOfFragmentLogParts(dataNodeId)
	expectReturnValue(t, err, uint64(returnValue32), 4)

}

func TestMgmClientImpl_GetConfigVersionFromNode(t *testing.T) {
	mci := getConnectionToMgmd(t)
	defer mci.Disconnect()

	dataNodeId := getAnyConnectedNodeId(t, mci, NodeTypeNDB)
	if dataNodeId == 0 {
		return
	}

	version, err := mci.GetConfigVersion(dataNodeId)
	if err != nil {
		t.Errorf("GetConfigVersionFromNode nodeId:%d failed : %s", dataNodeId, err)
	} else {
		t.Logf("Extracted config version: %d", version)
	}
}

// TestMgmClientImpl_getConfig_replyReceiver tests the
// getConfig reply handling with a fake server. The
// reply is broken up into two pieces to verify that
// it is properly being read by the executeCommand method.
func TestMgmClientImpl_getConfig_replyReceiver(t *testing.T) {
	mgmServer, mci := newFakeMgmServerAndClient(t)
	defer mci.Disconnect()
	defer mgmServer.disconnect()

	// Start the fake mgmd server with a get config reply
	mgmServer.run([]byte(`get config reply
result: Ok
Content-Length: 3271
Content-Type: ndbconfig/octet-stream
Content-Transfer-Encoding: base64

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
Ap4AAAAAIAACnwAAAA9TdGF0aWNTcGlu`),
		[]byte(`bmluZwAAEAACoQAAAAAQAAKiAAAAAhAAAqMAAAABEAAC
pAAAAAAQAAKlAAAAABAAAqYAAOpgEAACpwAAAAAQAAMmAAAAAAAAACMAAAAPAAAAAiAAAAUAAAAB
AAAAABAAAAkAAAAAEAAACwAAAAAQAADIAAAAABAAAMkAAAAAQAAAywAAAAAAAAAAEAACggAAAAAQ
AAMgAAQAABAAAyEAAEAAEAADIgAAAQAQAAMjAAAAARAAAyUAAAABEAADJgAAAAAQAAMnAAAF3BAA
AygAAAAAAAAAHAAAAAoAAAADIAAABQAAAApsb2NhbGhvc3QAAAAgAAAHAAAAAQAAAAAQAAAJAAAA
ABAAAAsAAAAAEAAAyAAAAAEQAADJAAAAAEAAAMsAAAAAAAAAABAAAMwAAAXcEAABLAAABKIQAAKC
AAAAAAAAACsAAAASAAAABBAAAZIAAAABEAABkwAAAAAQAAGWAAAAACAAAZcAAAABAAAAACAAAZgA
AAAKbG9jYWxob3N0AAAAEAABmQAAADcQAAGaAAAAAxAAAZsAAAAAEAABnAAAAAAQAAGdAAAAABAA
AZ4AAAAEEAABxgAgAAAQAAHHACAAABAAAckAAAAAEAABygAAAAAQAAHLAAAAABAAAcwAAAAAEAAB
zQAAAAAAAAADAAAAAAAAAAUAAAAOAAAAAwAAAAYQAAABAAAAMhAAAAIAAAABIAAAAwAAABJNQ18y
MDIxMTIyMjAwMzU1MQAAAAAAABsAAAADAAAAARAAAAMAAAACIAAABwAAACMvaG9tZS9teXNxbC9k
YXRhZGlyL25kYl9kYXRhL25vZGUyAAAgAAB9AAAAIy9ob21lL215c3FsL2RhdGFkaXIvbmRiX2Rh
dGEvbm9kZTIAAAAAAAUAAAABAAAAARAAAAMAAAADAAAABQAAAAEAAAADEAAAAwAAADIAAAAFAAAA
AQAAAAIQAAADAAAAkQAAAAUAAAABAAAAAhAAAAMAAACSAAAABQAAAAEAAAACEAAAAwAAAJMAAAAF
AAAAAQAAAAIQAAADAAAAlAAAAAUAAAABAAAAAhAAAAMAAACVAAAABQAAAAEAAAACEAAAAwAAAJYA
AAAFAAAAAQAAAAIQAAADAAAAlwAAAAUAAAABAAAAAhAAAAMAAACYAAAABQAAAAEAAAACEAAAAwAA
AJkAAAAFAAAAAQAAAAIQAAADAAAAmhVv0RY=
`))

	// Extract config through the management client connected
	// to the fake management server and verify the value
	value, err := mci.getConfig(
		1, cfgSectionTypeSystem, sysCfgConfigGenerationNumber, true)
	if err != nil {
		t.Fatalf("getConfig failed : %s", err)
	}

	// Check if the right config version is returned
	if value.(uint32) != 1 {
		t.Errorf("getConfig returned a wrong config version. Expected : 1. Recieved : %d", value.(uint32))
	}
}

// getFreeAPINodeId returns a free API NodeId
func getFreeAPINodeId(t *testing.T, mci *mgmClientImpl) int {
	status, err := mci.GetStatus()
	if err != nil {
		t.Errorf("GetStatus failed : %s", err)
	}

	for nodeId, nodeStatus := range status {
		if nodeStatus.IsAPINode() && !nodeStatus.IsConnected {
			return nodeId
		}
	}

	// No API node is free
	return 0
}

func TestMgmClientImpl_TryReserveNodeId(t *testing.T) {
	mci := getConnectionToMgmd(t)
	defer mci.Disconnect()

	freeAPINodeId := getFreeAPINodeId(t, mci)
	if freeAPINodeId == 0 {
		t.Skipf("No free API node")
	}

	// Try reserving the freeAPINode
	if _, err := mci.TryReserveNodeId(freeAPINodeId, NodeTypeAPI); err != nil {
		t.Errorf("Failed to reserve nodeId : %s", err)
	}

	// try again and expect an "already allocated" error
	expectedError := fmt.Sprintf("Id %d already allocated by another node.", freeAPINodeId)
	if _, err := mci.TryReserveNodeId(freeAPINodeId, NodeTypeAPI); err == nil {
		t.Errorf("Expected the second TryReserveNodeId to fail")
	} else if err.Error() != expectedError {
		t.Errorf("TryReserveNodeId returned an unexpected error : %s", err.Error())
	}
}
