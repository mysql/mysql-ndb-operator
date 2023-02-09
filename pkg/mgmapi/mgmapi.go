// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	klog "k8s.io/klog/v2"

	"github.com/mysql/ndb-operator/config/debug"
)

// MgmClient defines an interface that communicates with MySQL Cluster.
type MgmClient interface {
	Disconnect()
	GetStatus() (ClusterStatus, error)
	StopNodes(nodeIds []int) error
	TryReserveNodeId(nodeId int, nodeType NodeTypeEnum) (int, error)
	CreateNodeGroup(nodeIds []int) (int, error)

	GetConfigVersion(nodeID ...int) (uint32, error)
	GetDataMemory(dataNodeId int) (uint64, error)
	GetMgmdArbitrationRank() (uint32, error)
	GetMaxNoOfTables(dataNodeId int) (uint32, error)
	GetMaxNoOfAttributes(dataNodeId int) (uint32, error)
	GetMaxNoOfOrderedIndexes(dataNodeId int) (uint32, error)
	GetMaxNoOfUniqueHashIndexes(dataNodeId int) (uint32, error)
	GetMaxNoOfConcurrentOperations(dataNodeId int) (uint32, error)
	GetTransactionBufferMemory(dataNodeId int) (uint32, error)
	GetIndexMemory(dataNodeId int) (uint64, error)
	GetRedoBuffer(dataNodeId int) (uint32, error)
	GetLongMessageBuffer(dataNodeId int) (uint32, error)
	GetDiskPageBufferMemory(dataNodeId int) (uint64, error)
	GetSharedGlobalMemory(dataNodeId int) (uint64, error)
	GetTransactionMemory(dataNodeId int) (uint64, error)
	GetNoOfFragmentLogParts(dataNodeId int) (uint32, error)
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
		err = debug.InternalError("wrong usage of optional desiredNodeId parameter")
	}

	if err != nil {
		klog.Errorf("Error connecting management server : %s", err)
		return nil, err
	}
	return client, nil
}

// connect creates a tcp connection to the mgmd server
// Note : always use NewMgmClient to create a client rather
// than directly using mgmClientImpl and connect
func (mci *mgmClientImpl) connect(connectstring string) error {

	// Parse the addresses from connectstring and dial them one
	// by one until a connection can be established.
	// Connectstring can be of form :
	// [nodeid=node_id, ]host-definition[, host-definition[, ...]]
	//
	// host-definition:
	//    host_name[:port_number]
	//
	// Note : connection string with bind-address is not supported
	var err error
	for _, host := range strings.Split(connectstring, ",") {
		if strings.HasPrefix(host, "nodeid=") {
			// Ignore this token
			continue
		}

		mci.connection, err = net.Dial("tcp", host)
		if err != nil {
			klog.V(4).Infof("Failed to connect to Management Node at %q : %s", host, err)
			// Try the next host in the connectstring
			continue
		}
		klog.V(4).Infof("Management server connected to node at %q", host)
		break
	}

	return err
}

// connectToNodeId creates a tcp connection to the mgmd with the given id
// Note : always use NewMgmClient to create a client rather
// than directly using mgmClientImpl and connectToNodeId
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
)

// executeCommand sends the command to the Management Server,
// reads back the reply, parses it and returns the reply and
// other details to the caller.
func (mci *mgmClientImpl) executeCommand(
	command string, args map[string]interface{},
	slowCommand bool, expectedReplyDetails []string) (map[string]string, error) {

	if mci.connection == nil {
		return nil, debug.InternalError("MgmClient is not connected to Management server")
	}

	// Build the command and args
	var cmdWithArgs bytes.Buffer
	fmt.Fprintf(&cmdWithArgs, "%s\n", command)
	for arg, value := range args {
		fmt.Fprintf(&cmdWithArgs, "%s: %v\n", arg, value)
	}
	// mark end of command and args
	fmt.Fprint(&cmdWithArgs, "\n")

	// Set write deadline
	err := mci.connection.SetWriteDeadline(time.Now().Add(defaultReadWriteTimeout))
	if err != nil {
		klog.Error("SetWriteDeadline failed : ", err)
		return nil, err
	}

	// Send the command along with the args to the connected mgmd server
	if _, err = mci.connection.Write(cmdWithArgs.Bytes()); err != nil {
		klog.Error("failed to send command to connected management server :", err)
		return nil, err
	}

	// Pick and set a timeout for the read
	replyReadTimeout := defaultReadWriteTimeout
	if slowCommand {
		replyReadTimeout = delayedReplyTimeout
	}
	err = mci.connection.SetReadDeadline(time.Now().Add(replyReadTimeout))
	if err != nil {
		klog.Error("SetReadDeadline failed : ", err)
		return nil, err
	}

	// Start reading the reply via a scanner
	scanner := bufio.NewScanner(mci.connection)

	// Setup defer to flush out all the reply from the connection in case of any errors
	readDone := false
	defer func() {
		if !readDone && scanner.Err() == nil {
			for scanner.Scan() && scanner.Text() != "" {
			}
		}
	}()

	// Read and verify the reply header.
	// note : most parse errors are development errors, so panic in debug builds
	if scanner.Scan() {
		header := scanner.Text()

		// Check if the header has the expected reply header
		if len(expectedReplyDetails) == 0 {
			return nil, debug.InternalError("expectedReplyDetails is empty")
		}
		if header != expectedReplyDetails[0] {
			// Unexpected header. Check for common format errors
			if strings.HasPrefix(header, "result: ") {
				// The Management Server has sent back a bad protocol error
				tokens := strings.SplitN(header, ":", 2)
				return nil, debug.InternalError("executeCommand failed :" + tokens[1])
			}

			// Unrecognizable header. Log details and return error.
			klog.Errorf("Expected header : %s, Actual header : %s",
				expectedReplyDetails[0], header)
			return nil, debug.InternalError("unexpected header in reply")
		}

	}

	// Read and parse the rest of the reply which has the details.
	replyDetails := make(map[string]string)
	for scanner.Scan() {
		// read the line
		replyLine := scanner.Text()

		if replyLine == "" {
			// Empty line marks the end of reply.
			// Stop reading.
			break
		}

		// extract the information
		tokens := strings.SplitN(replyLine, ":", 2)

		if len(tokens) != 2 {
			// The reply detail is not of form key:value.
			// Usage issue or MySQL Cluster has updated its wire protocol.
			// In any case, panic to bring in early attention.
			klog.Error("reply has unexpected format : ", replyLine)
			return nil, debug.InternalError("missing colon(:) in reply detail")
		}

		// store the key value pair
		replyDetails[tokens[0]] = strings.TrimSpace(tokens[1])
	}

	// The reply has been read and if the method returns
	// after this point, no need to read more from the
	// connection inside the 'defer' logic.
	readDone = true

	// Handle any error thrown by the scanner
	if err = scanner.Err(); err != nil {
		klog.Error("failed to read reply from Management Server :", err)
		return nil, err
	}

	// if the reply details has a 'Content-Length' key, the 'Content' appended to the reply has to be read
	if contentLengthVal, exists := replyDetails["Content-Length"]; exists {
		contentLength, err := strconv.Atoi(contentLengthVal)
		if err != nil {
			return nil, err
		}

		// So far only the get config command expects a content in
		// the reply and the maximum length of that is 1024*1024 bytes
		if contentLength == 0 || contentLength > 1024*1024 {
			return nil, debug.InternalError("unexpected content length in reply")
		}
		// Increment contentLength to account for the trailing \n
		contentLength += 1

		// Read the content
		var content string
		for len(content) < contentLength && scanner.Scan() {
			data := scanner.Text()
			content += data + "\n"
		}

		// Handle any error thrown by the scanner
		if err = scanner.Err(); err != nil {
			klog.Error("failed to read content from Management Server :", err)
			return nil, err
		}
		if len(content) != contentLength {
			return nil, debug.InternalError("mismatched content and content-length in reply")
		}

		// Add the content to replyDetails
		replyDetails["Content"] = content
	}

	// Validate the reply details.
	// If there is a 'result' key, check if it is 'Ok'
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
			return nil, debug.InternalError("expected detail not found in reply")
		}
	}

	return replyDetails, nil
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
		return nil, debug.InternalError("'nodes' value in node status reply unexpected format")
	}
	delete(reply, "nodes")

	// loop all values in reply and put them in a ClusterStatus object
	cs := NewClusterStatus(nodeCount)
	for aggregateKey, value := range reply {
		// the key is of form node.3.version
		keyTokens := strings.SplitN(aggregateKey, ".", 3)
		if len(keyTokens) != 3 || keyTokens[0] != "node" {
			return nil, debug.InternalError("node status reply has unexpected format")
		}

		// extract node id and the actual key
		nodeId, err := strconv.Atoi(keyTokens[1])
		if err != nil {
			return nil, debug.InternalError("node status reply has unexpected format")
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
						return nil, debug.InternalError("node_group in node status reply has unexpected format")
					}
				} else {
					// NodeGroup is not set in reply as the node is not connected yet.
					// Retrieve the NodeGroup using 'get config' command
					nodeGroupValue, err := mci.getConfig(nodeId, cfgSectionTypeNDB, dbCfgNodegroup, true)
					if err != nil {
						return nil, debug.InternalError("node_group was not found in data node config " + keyTokens[1])
					}
					ns.NodeGroup = int(nodeGroupValue.(uint32))
				}
			}

		case "version":
			versionNumber, err := strconv.Atoi(value)
			if err != nil {
				return nil, debug.InternalError("version in node status reply has unexpected format")
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
	_, err := mci.executeCommand(
		"stop v2", args, true,
		[]string{"stop reply", "result", "stopped", "disconnect"})
	if err != nil {
		return err
	}

	return nil
}

// TryReserveNodeId attempts to temporarily reserve the given nodeId of nodeType
// for a second. It returns reserved nodeId on success and an error on failure.
// This is used by the various MySQL Cluster node pods' init containers to check
// if the existing nodes are ready to accept a connection from the node.
func (mci *mgmClientImpl) TryReserveNodeId(nodeId int, nodeType NodeTypeEnum) (int, error) {

	// command : (note : version, user, password and public key are mandatory but ignored by the Management Server)
	// get nodeid
	// version: <composite ndb version of the client>
	// nodeid: <nodeId>
	// nodetype: <nodeType>
	// timeout: <seconds after which the nodeId reservation expires>  nbb
	// user: dummy
	// password: dummy
	// public key: dummy

	// get nodeid reply
	// nodeid: <Assigned nodeId>
	// result: Ok

	// build args
	args := map[string]interface{}{
		// version is read by the server but ultimately not used
		"version":  0,
		"nodeid":   nodeId,
		"nodetype": nodeType,
		// reserve nodeId only for a second
		"timeout": 1,
		// user, password and public key are ignored by the server
		"user":       "",
		"password":   "",
		"public key": "",
	}

	// send the command and return the assigned nodeId
	reply, err := mci.executeCommand(
		"get nodeid", args, false,
		[]string{"get nodeid reply", "nodeid"})
	if err != nil {
		return 0, err
	}

	reservedNodeId, err := strconv.Atoi(reply["nodeid"])
	if err != nil {
		return 0, debug.InternalError("nodeid in get nodeid reply has unexpected format : " + err.Error())
	}

	return reservedNodeId, nil
}

// CreateNodeGroup creates a nodegroup with nodes with the given nodeIds
func (mci *mgmClientImpl) CreateNodeGroup(nodeIds []int) (int, error) {

	// command :
	// create nodegroup
	// nodes: <nodeId seperated by blank space>

	// reply :
	// create nodegroup reply
	// result: Ok
	// ng: <nodegroup ID>

	// build args
	nodeList := fmt.Sprintf("%d", nodeIds[0])
	for i := 1; i < len(nodeIds); i++ {
		nodeList += fmt.Sprintf(" %d", nodeIds[i])
	}

	args := map[string]interface{}{
		"nodes": nodeList,
	}

	// send the command and read the reply
	reply, err := mci.executeCommand(
		"create nodegroup", args, true,
		[]string{"create nodegroup reply", "result", "ng"})
	if err != nil {
		return -1, err
	}

	ng, err := strconv.Atoi(reply["ng"])
	if err != nil {
		return 0, debug.InternalError("nodegroup in create nodegroup reply has unexpected format : " + err.Error())
	}

	return ng, nil
}

// getConfig extracts the value of the config variable 'configKey'
// from the MySQL Cluster node with node id 'nodeId'. The config
// is either retrieved from the config stored in connected
// Management node or from the config stored in the node with 'nodeId' itself.
func (mci *mgmClientImpl) getConfig(
	nodeId int, sectionFilter cfgSectionType, configKey uint32, getConfigFromConnectedMgmd bool) (configValue, error) {

	// command : (note : version is mandatory but ignored by the Management Server)
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
		"Content",
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
		return nil, debug.InternalError("unexpected content in get config reply")
	}

	// extract the required config value from the base64 encoded config data
	value := readConfigFromBase64EncodedData(reply["Content"], sectionFilter, uint32(nodeId), configKey)
	if value == nil {
		return nil, debug.InternalError("getConfig failed")
	}

	return value, nil
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
		return 0, debug.InternalError("wrong usage of GetConfigVersion arguments")
	}

	value, err := mci.getConfig(fromNodeId, cfgSectionTypeSystem, sysCfgConfigGenerationNumber, false)
	if err != nil {
		return 0, err
	}
	return value.(uint32), nil
}

// GetMgmdArbitrationRank returns the arbitration rank of the connected mgmd node
func (mci *mgmClientImpl) GetMgmdArbitrationRank() (uint32, error) {
	value, err := mci.getConfig(0, cfgSectionTypeMGM, nodeCfgArbitRank, true)
	if err != nil {
		return 0, err
	}
	return value.(uint32), nil
}

// getDataNodeConfigValueUint32 returns the value of the uint32 dbCfgKey config of the data node with id dataNodeId
func (mci *mgmClientImpl) getDataNodeConfigValueUint32(dataNodeId int, dbCfgKey uint32) (uint32, error) {
	value, err := mci.getConfig(dataNodeId, cfgSectionTypeNDB, dbCfgKey, false)
	if err != nil {
		return 0, err
	}
	return value.(uint32), nil
}

// getDataNodeConfigValueUint64 returns the value of the uint64 dbCfgKey config of the data node with id dataNodeId
func (mci *mgmClientImpl) getDataNodeConfigValueUint64(dataNodeId int, dbCfgKey uint32) (uint64, error) {
	value, err := mci.getConfig(dataNodeId, cfgSectionTypeNDB, dbCfgKey, false)
	if err != nil {
		return 0, err
	}
	return value.(uint64), nil
}

// GetDataMemory returns the data memory of the datanode with id dataNodeId
func (mci *mgmClientImpl) GetDataMemory(dataNodeId int) (uint64, error) {
	return mci.getDataNodeConfigValueUint64(dataNodeId, dbCfgDataMemory)
}

// GetMaxNoOfTables returns the maximum number of tables as per the datanode with id dataNodeId
func (mci *mgmClientImpl) GetMaxNoOfTables(dataNodeId int) (uint32, error) {
	return mci.getDataNodeConfigValueUint32(dataNodeId, dbCfgNoTables)
}

// GetMaxNoOfAttributes returns the maximum number of attributes as per the datanode with id dataNodeId
func (mci *mgmClientImpl) GetMaxNoOfAttributes(dataNodeId int) (uint32, error) {
	return mci.getDataNodeConfigValueUint32(dataNodeId, dbCfgNoAttributes)
}

// GetMaxNoOfOrderedIndexes returns the maximum number of ordered indexes as per the datanode with id dataNodeId
func (mci *mgmClientImpl) GetMaxNoOfOrderedIndexes(dataNodeId int) (uint32, error) {
	return mci.getDataNodeConfigValueUint32(dataNodeId, dbCfgNoOrderedIndexes)
}

// GetMaxNoOfUniqueHashIndexes returns the maximum number of unique hash indexes as per the datanode with id dataNodeId
func (mci *mgmClientImpl) GetMaxNoOfUniqueHashIndexes(dataNodeId int) (uint32, error) {
	return mci.getDataNodeConfigValueUint32(dataNodeId, dbCfgNoUniqueHashIndexes)
}

// GetMaxNoOfConcurrentOperations returns the maximum number of concurrent operations as per the datanode with id dataNodeId
func (mci *mgmClientImpl) GetMaxNoOfConcurrentOperations(dataNodeId int) (uint32, error) {
	return mci.getDataNodeConfigValueUint32(dataNodeId, dbCfgNoOps)
}

// GetTransactionBufferMemory returns the transaction buffer memory of the datanode with id dataNodeId
func (mci *mgmClientImpl) GetTransactionBufferMemory(dataNodeId int) (uint32, error) {
	return mci.getDataNodeConfigValueUint32(dataNodeId, dbCfgTransBufferMem)
}

// GetIndexMemory returns the index memory of the datanode with id dataNodeId
func (mci *mgmClientImpl) GetIndexMemory(dataNodeId int) (uint64, error) {
	return mci.getDataNodeConfigValueUint64(dataNodeId, dbCfgIndexMem)
}

// GetRedoBuffer returns the redo buffer of the datanode with id dataNodeId
func (mci *mgmClientImpl) GetRedoBuffer(dataNodeId int) (uint32, error) {
	return mci.getDataNodeConfigValueUint32(dataNodeId, dbCfgRedoBuffer)
}

// GetLongMessageBuffer returns the message buffer of the datanode with id dataNodeId
func (mci *mgmClientImpl) GetLongMessageBuffer(dataNodeId int) (uint32, error) {
	return mci.getDataNodeConfigValueUint32(dataNodeId, dbCfgLongSignalBuffer)
}

// GetDiskPageBufferMemory returns the disk page buffer memory of the datanode with id dataNodeId
func (mci *mgmClientImpl) GetDiskPageBufferMemory(dataNodeId int) (uint64, error) {
	return mci.getDataNodeConfigValueUint64(dataNodeId, dbCfgDiskPageBufferMemory)
}

// GetSharedGlobalMemory returns the shared global memory of the datanode with id dataNodeId
func (mci *mgmClientImpl) GetSharedGlobalMemory(dataNodeId int) (uint64, error) {
	return mci.getDataNodeConfigValueUint64(dataNodeId, dbCfgSga)
}

// GetTransactionMemory returns the transaction memory of the datanode with id dataNodeId
func (mci *mgmClientImpl) GetTransactionMemory(dataNodeId int) (uint64, error) {
	return mci.getDataNodeConfigValueUint64(dataNodeId, dbCfgTransactionMemory)
}

// GetNoOfFragmentLogParts returns the number of fragment log parts in the datanode with id dataNodeId
func (mci *mgmClientImpl) GetNoOfFragmentLogParts(dataNodeId int) (uint32, error) {
	return mci.getDataNodeConfigValueUint32(dataNodeId, dbCfgNoRedologParts)
}
