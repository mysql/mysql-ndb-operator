// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"k8s.io/klog"
)

// MgmClient defines an interface that communicates with MySQL Cluster.
type MgmClient interface {
	Disconnect()
	GetStatus() (ClusterStatus, error)
	GetConfigVersion(nodeID ...int) (uint32, error)
	StopNodes(nodeIds []int) error
	GetDataMemory(dataNodeId int) (uint64, error)
}

// mgmClientImpl implements the MgmClient interface
// It opens up a connection and communicates with the
// MySQL Cluster nodes via wire protocol.
type mgmClientImpl struct {
	connection net.Conn
}

// NewMgmClient returns a new mgmClientImpl connected to MySQL Cluster
func NewMgmClient(connectstring string, desiredNodeId ...int) (*mgmClientImpl, error) {

	client := &mgmClientImpl{}
	var err error
	switch len(desiredNodeId) {
	case 0:
		// connect to any nodeId
		err = client.connect(connectstring)
	case 1:
		// connect to the given desired nodeId
		err = client.connectToNodeId(connectstring, desiredNodeId[0])
	default:
		panic("wrong usage of optional desiredNodeId parameter")
	}

	if err != nil {
		klog.Errorf("Error connecting management server : %s", err)
		return nil, err
	}
	return client, nil
}

// connect creates a tcp connection to the mgmd server
// Note : always use NewMgmClient to create a client rather
//        than directly using mgmClientImpl and connect
func (mci *mgmClientImpl) connect(connectstring string) error {
	var err error
	mci.connection, err = net.Dial("tcp", connectstring)
	if err != nil {
		mci.connection = nil
		return err
	}

	klog.V(4).Infof("Management server connected.")
	return nil
}

// connectToNodeId creates a tcp connection to the mgmd with the given id
// Note : always use NewMgmClient to create a client rather
//        than directly using mgmClientImpl and connectToNodeId
func (mci *mgmClientImpl) connectToNodeId(connectstring string, desiredNodeId int) error {

	var lastDNSError error
	// The operator might be running from outside the K8s cluster in which case,
	// it will not be possible to connect to desired mgmd in a single attempt.
	// So, attempt connecting to the given connectstring, which probably is the
	// load balancer URL and check the id of the connected node. If it is not the
	// desired node id, retry.
	for retries := 0; retries < 10; retries++ {
		err := mci.connect(connectstring)
		if err != nil {
			if _, ok := err.(*net.DNSError); ok {
				// Server not available yet. The pod is probably
				// not up or the load balancer is not up. Retry.
				klog.Error("Management server is not available yet")
				lastDNSError = err
				continue
			}

			// Some other error connecting to the server
			klog.Errorf("Failed to connect to management server : %s", err)
			return err
		}

		// Connected to an mgmd. Check if it is the desired one.
		lastDNSError = nil
		nodeId, err := mci.getConnectedMgmdNodeId()
		if err != nil {
			mci.Disconnect()
			klog.Errorf("Failed to retrieve connected management server node id : %s", err)
			return err
		}

		if nodeId == desiredNodeId {
			// found the one
			return nil
		}
		mci.Disconnect()
	}

	// Failed to connect to the right management node or
	// due to the host not available
	if lastDNSError == nil {
		return errors.New("failed to connect to the desired nodeId")
	}
	return lastDNSError
}

// Disconnect closes the tcp connection to the mgmd server
func (mci *mgmClientImpl) Disconnect() {
	if mci.connection != nil {
		_ = mci.connection.Close()
		klog.V(4).Infof("Management server disconnected.")
	}
}

const (
	// defaultReadWriteTimeout is the default read and write timeout
	// used by the TCP connection when executing a command.
	defaultReadWriteTimeout = 5 * time.Second

	// delayedReplyTimeout is the reply read timeout used for
	// commands that are expected to take a long time to complete.
	// Like a restart data node or a start backup command.
	// For most other cases like a config lookup or node status lookup,
	// the defaultReadWriteTimeout should be sufficient.
	delayedReplyTimeout = 300 * time.Second

	// maxReadRetries is the maximum times reply read will be retried
	maxReadRetries = 6
)

// sendCommand sends the given command and args to the
// connected management server and reads back the reply.
// Note : it might be easier and will be sufficient for most
// cases to use executeCommand rather than directly calling
// sendCommand and parseReply
func (mci *mgmClientImpl) sendCommand(
	command string, args map[string]interface{}, slowCommand bool) ([]byte, error) {

	if mci.connection == nil {
		panic("MgmClient is not connected to Management server")
	}

	// Build the command and args
	var cmdWithArgs bytes.Buffer
	fmt.Fprintf(&cmdWithArgs, "%s\n", command)
	for arg, value := range args {
		fmt.Fprintf(&cmdWithArgs, "%s: %v\n", arg, value)
	}
	// mark end of command and args
	fmt.Fprint(&cmdWithArgs, "\n")

	// Set a write deadline
	err := mci.connection.SetWriteDeadline(time.Now().Add(defaultReadWriteTimeout))
	if err != nil {
		klog.Error("SetWriteDeadline failed : ", err)
		return nil, err
	}

	// Send the command along with the args to the connected mgmd server
	if _, err = mci.connection.Write(cmdWithArgs.Bytes()); err != nil {
		klog.Error("failed to send command to connected management server : ", err)
		return nil, err
	}

	// Set a reply read deadline
	replyReadTimeout := defaultReadWriteTimeout
	if slowCommand {
		replyReadTimeout = delayedReplyTimeout
	}
	err = mci.connection.SetReadDeadline(time.Now().Add(replyReadTimeout))
	if err != nil {
		klog.Error("SetReadDeadline failed : ", err)
		return nil, err
	}

	// Read back the reply
	// TODO: check if retry is needed?
	retries := 0
	var reply []byte
	buffer := make([]byte, 64)
	for {
		if retries > 0 {
			klog.Infof("read retry # %d", retries)
		}

		n, err := mci.connection.Read(buffer)
		if err != nil {
			// Error occurred during read
			if retries > maxReadRetries || err == io.EOF {
				// no more retries or the connection closed
				return nil, err
			}
			retries++
			continue
		}

		// append the read bytes to reply
		reply = append(reply, buffer[:n]...)
		if n < 64 {
			// done reading reply
			return reply, nil
		}
	}
}

// parseReply parses the reply sent by the management server,
// validates and extracts the information from it.
// Note : it might be easier and will be sufficient for most
// cases to use executeCommand rather than directly calling
// sendCommand and parseReply
func (mci *mgmClientImpl) parseReply(
	command string, reply []byte, expectedReplyDetails []string) (map[string]string, error) {
	// create a scanner and start parsing reply
	scanner := bufio.NewScanner(bytes.NewReader(reply))

	// read and verify the header
	// note : most parse errors are development errors.
	//        so, panic to help faster debugging
	if scanner.Scan() {
		header := scanner.Text()

		// check for common command format errors
		for _, err := range []string{
			"Unknown command",
			"Unknown argument",
			"Missing arg",
			"Type mismatch",
		} {
			if strings.HasPrefix(header, "result: "+err) {
				// protocol usage error
				panic("sendCommand failed : " + err)
			}
		}

		// check if it has the expected reply header
		if len(expectedReplyDetails) == 0 {
			panic("expectedReplyDetails is empty")
		}
		if header != expectedReplyDetails[0] {
			klog.Errorf("Expected header : %s, Actual header : %s",
				expectedReplyDetails[0], header)
			panic("unexpected header in reply")
		}
	}

	// Extract and store the details sent as a part of the reply
	replyDetails := make(map[string]string)
	for scanner.Scan() {
		// read the line
		replyLine := scanner.Text()

		if len(replyLine) == 0 {
			// ignore blank spaces
			klog.V(4).Info("ignoring blank space")
			continue
		}

		// extract the information
		tokens := strings.SplitN(replyLine, ":", 2)

		if len(tokens) != 2 {
			// The reply detail is not of form key:value
			// Check if this is a 'get config reply', in that case,
			// read in the octet stream at the end of the reply
			if expectedReplyDetails[0] == "get config reply" {
				// everything starting from here is the config binary data
				config := scanner.Text()
				for scanner.Scan() {
					//read until the end
					config += scanner.Text()
				}

				// store it in the reply
				replyDetails["config"] = config
				// end of reply
				break
			}

			// Usage issue or MySQL Cluster has updated its wire protocol.
			// In any case, panic to bring in early attention.
			klog.Error("reply has unexpected format : ", replyLine)
			panic("missing colon(:) in reply detail")
		}

		// store it
		replyDetails[tokens[0]] = strings.TrimSpace(tokens[1])
	}

	// handle any errors in scanner
	if err := scanner.Err(); err != nil {
		klog.Error("parsing reply failed : ", err)
		return nil, err
	}

	// Validate the reply details.
	// If there is an 'result' key, check if it is 'Ok'
	if result, exists := replyDetails["result"]; exists && result != "Ok" {
		// Command failed
		return nil, errors.New(result)
	}

	// Check if expected details are present.
	// if expectedReplyDetails is nil, validation is skipped.
	for i := 1; i < len(expectedReplyDetails); i++ {
		expectedDetail := expectedReplyDetails[i]
		if _, exists := replyDetails[expectedDetail]; !exists {
			// expected detail not present
			klog.Errorf("Expected detail '%s' not found in reply", expectedDetail)
			panic("expected detail not found in reply")
		}
	}

	return replyDetails, nil
}

// executeCommand sends the command, reads the reply, parses it and returns
func (mci *mgmClientImpl) executeCommand(
	command string, args map[string]interface{},
	slowCommand bool, expectedReplyDetails []string) (map[string]string, error) {

	// send the command and get reply
	reply, err := mci.sendCommand(command, args, slowCommand)
	if err != nil {
		return nil, err
	}

	// parse the reply and return
	return mci.parseReply(command, reply, expectedReplyDetails)
}

// getConnectedMgmdNodeId returns the nodeId of the mgmd
// to which the tcp connection is established
func (mci *mgmClientImpl) getConnectedMgmdNodeId() (int, error) {

	// command :
	// get mgmd nodeid

	// reply :
	// get mgmd nodeid reply
	// nodeid:%u

	// send the command and read the reply
	reply, err := mci.executeCommand(
		"get mgmd nodeid", nil, false,
		[]string{"get mgmd nodeid reply", "nodeid"})
	if err != nil {
		return 0, err
	}

	connectedNodeId, err := strconv.Atoi(reply["nodeid"])
	if err != nil {
		klog.Error("failed to parse the returned nodeid")
		return 0, err
	}

	return connectedNodeId, nil
}

// GetStatus reads the MySQL Cluster nodes' status and returns
// a ClusterStatus object filled with that information
func (mci *mgmClientImpl) GetStatus() (ClusterStatus, error) {

	// command :
	// get status

	// reply :
	// node status
	// nodes: 13
	// node.2.type: NDB
	// node.2.status: STARTED
	// node.2.version: 524314
	// .....

	// send the command and read the reply
	reply, err := mci.executeCommand(
		"get status", nil, false,
		[]string{"node status", "nodes"})
	if err != nil {
		return nil, err
	}

	nodeCount, err := strconv.Atoi(reply["nodes"])
	if err != nil {
		klog.Error("failed to parse nodes value in node status reply : ", err)
		panic("'nodes' value in node status reply unexpected format")
	}
	delete(reply, "nodes")

	// loop all values in reply and put them in a ClusterStatus object
	cs := NewClusterStatus(nodeCount)
	for aggregateKey, value := range reply {
		// the key is of form node.3.version
		keyTokens := strings.SplitN(aggregateKey, ".", 3)
		if len(keyTokens) != 3 || keyTokens[0] != "node" {
			panic("node status reply has unexpected format")
		}

		// extract node id and the actual key
		nodeId, err := strconv.Atoi(keyTokens[1])
		if err != nil {
			panic("node status reply has unexpected format")
		}
		key := keyTokens[2]

		// update nodestatus based on the key
		ns := cs.ensureNode(nodeId)
		switch key {
		case "type":
			ns.setNodeTypeFromTLA(value)

			// Read and set status here.
			// This is done here and not in a separate case as
			// setting status depends on the type and if it is
			// handled in a separate case, there is no guarantee
			// that the type would have been set, due to the
			// arbitrary looping order of the 'reply' map.
			statusKey := fmt.Sprintf("node.%d.status", nodeId)
			statusValue := reply[statusKey]
			// for data node, STARTED => connected
			// for mgm/api nodes, CONNECTED => connected
			if (ns.IsDataNode() && statusValue == "STARTED") ||
				(!ns.IsDataNode() && statusValue == "CONNECTED") {
				ns.IsConnected = true
			}

			// In a similar manner, set node group for the data node. It is set
			// in get status reply only if the data node is connected.
			if ns.IsDataNode() {
				if ns.IsConnected {
					// Extract the NodeGroup from reply
					nodeGroupKey := fmt.Sprintf("node.%d.node_group", nodeId)
					ns.NodeGroup, err = strconv.Atoi(reply[nodeGroupKey])
					if err != nil {
						panic("node_group in node status reply has unexpected format")
					}
				} else {
					// NodeGroup is not set in reply as the node is not connected yet.
					// Retrieve the NodeGroup using 'get config' command
					nodeGroupValue, err := mci.getConfig(nodeId, cfgSectionTypeNDB, dbCfgNodegroup, true)
					if err != nil {
						panic("node_group was not found in data node config " + keyTokens[1])
					}
					ns.NodeGroup = int(nodeGroupValue.(uint32))
				}
			}

		case "version":
			versionNumber, err := strconv.Atoi(value)
			if err != nil {
				panic("version in node status reply has unexpected format")
			}
			ns.SoftwareVersion = getMySQLVersionString(versionNumber)
		}
	}

	return cs, nil
}

// StopNodes sends a command to the Management Server to stop the requested nodes.
// On success, it returns nil and on failure, it returns an error
func (mci *mgmClientImpl) StopNodes(nodeIds []int) error {

	// command :
	// stop v2
	// node: <node list>
	// abort: 0
	// force: 0

	// reply :
	// stop reply
	// result: Ok
	// stopped: 1
	// disconnect: 0

	// build args
	nodeList := fmt.Sprintf("%d", nodeIds[0])
	for i := 1; i < len(nodeIds); i++ {
		nodeList += fmt.Sprintf(" %d", nodeIds[i])
	}

	args := map[string]interface{}{
		"node":  nodeList,
		"abort": 0,
		"force": 0,
	}

	// send the command and read the reply
	reply, err := mci.executeCommand(
		"stop v2", args, true,
		[]string{"stop reply", "result", "stopped", "disconnect"})
	if err != nil {
		return err
	}

	if reply["result"] != "Ok" {
		// stop failed
		return errors.New(reply["result"])
	}

	return nil
}

// getConfig extracts the value of the config variable 'configKey'
// from the MySQL Cluster node with node id 'nodeId'. The config
// is either retrieved from the config stored in connected
// Management node or from the config stored in the node with 'nodeId' itself.
func (mci *mgmClientImpl) getConfig(
	nodeId int, sectionFilter cfgSectionType, configKey uint32, getConfigFromConnectedMgmd bool) (configValue, error) {

	// command :
	// get config_v2
	// version: <Configuration version number>
	// node: <communication sections of the node to send back in reply>
	// nodetype: <Type of requesting node>
	// from_node: <Node to get config from>

	// reply :
	// get config reply
	// result: Ok
	// Content-Length: 4588
	// Content-Type: ndbconfig/octet-stream
	// Content-Transfer-Encoding: base64
	// <newline>
	// <config as octet stream>

	// build args
	args := map[string]interface{}{
		// version args is ignored by Mgm Server, but they are mandatory so just send 0
		"version": 0,
		// this client is not exactly an API node but should be okay to mimic one.
		"nodetype": NodeTypeAPI,
		// The 'node' arg can be set to one of the following value :
		// 0 - to receive all the communication sections in the config (or)
		// node id - to receive all communication sections related to node with the given id.
		// The management node seems to accept even a node id that doesn't exist in the config
		// and in that case, there are no communication sections as the node is non-existent.
		// In any case, the sent back communication sections will be ignored by this client.
		// So, set this arg to the maximum possible API node id, which has the least chance of
		// appearing in the config, to reduce the amount of data sent back.
		"node": 255,
	}

	if !getConfigFromConnectedMgmd {
		// Get the config directly from the node with 'nodeId'.
		args["from_node"] = nodeId
	}

	expectedReply := []string{
		"get config reply",
		"result",
		"Content-Length",
		"Content-Type",
		"Content-Transfer-Encoding",
		"config",
	}

	// send the command and read the reply
	reply, err := mci.executeCommand("get config_v2", args, false, expectedReply)
	if err != nil {
		return nil, err
	}

	if reply["result"] != "Ok" {
		// get config
		return nil, errors.New(reply["result"])
	}

	if reply["Content-Type"] != "ndbconfig/octet-stream" ||
		reply["Content-Transfer-Encoding"] != "base64" {
		panic("unexpected content in get config reply")
	}

	return readConfigFromBase64EncodedData(reply["config"], sectionFilter, uint32(nodeId), configKey), nil
}

// GetConfigVersion returns the config version of the node with nodeId. If nodeId
// is not given, it will return the config version of the connected Management Server.
func (mci *mgmClientImpl) GetConfigVersion(nodeId ...int) (uint32, error) {
	var fromNodeId int
	if len(nodeId) == 1 {
		fromNodeId = nodeId[0]
	}

	if len(nodeId) > 1 {
		// nodeId is meant to be an optional argument and not var args
		panic("wrong usage of GetConfigVersion arguments")
	}

	value, err := mci.getConfig(fromNodeId, cfgSectionTypeSystem, sysCfgConfigGenerationNumber, false)
	if err != nil {
		return 0, err
	}
	return value.(uint32), nil
}

// GetDataMemory returns the data memory of the datanode with id dataNodeId
func (mci *mgmClientImpl) GetDataMemory(dataNodeId int) (uint64, error) {
	value, err := mci.getConfig(dataNodeId, cfgSectionTypeNDB, dbCfgDataMemory, false)
	if err != nil {
		return 0, err
	}
	return value.(uint64), nil
}
