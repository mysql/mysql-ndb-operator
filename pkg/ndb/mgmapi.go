package ndb

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"k8s.io/klog"
)

const maxReadRetries = 6
const defaultTimeoutInSeconds = 5

const commandRestart = "restart node v2"

const CFG_SYS_CONFIG_GENERATION = 2

const InvalidTypeId = 0
const IntTypeId = 1
const StringTypeId = 2
const SectionTypeId = 3
const Int64TypeId = 4

type Mgmclient struct {
	connection net.Conn
}

type ConfigValueType interface{}
type ConfigSection map[uint32]ConfigValueType

func (api *Mgmclient) Disconnect() {
	if api.connection != nil {
		api.connection.Close()
		klog.Infof("Management server disconnected.")
	}
}

func (api *Mgmclient) Connect() error {

	// connect to server
	var err error
	api.connection, err = net.Dial("tcp", "127.0.0.1:1186")
	if err != nil {
		api.connection = nil
		return err
	}

	klog.Infof("Management server connected.")

	return nil
}

func (api *Mgmclient) ConnectToNodeId(wantedNodeId int) error {
	// as for external testing we are going through load balancer we
	// circle here until we got the right node
	var connectError error
	connected := false

	for retries := 0; retries < 10; retries++ {

		connectError = nil
		err := api.Connect()
		if err != nil {
			// TODO: if we do not have contact then this could be a permanent issue
			//   like a config error (bug in operator)

			// but in most cases this is rather due to e.g. pod starting or being created
			klog.Errorf("No contact to management server at the moment")

			// so we retry
			connectError = err
			continue
		}

		nodeid, err := api.GetOwnNodeId()
		if err != nil {
			api.Disconnect()
			connectError = err
			continue
		}

		if nodeid == wantedNodeId {
			connected = true
			break
		}
		api.Disconnect()
	}

	if !connected {
		if connectError != nil {
			return connectError
		}
		s := fmt.Sprintf("Failed to connect to correct management server with node id %d", wantedNodeId)
		return errors.New(s)
	}

	return nil
}

func (api *Mgmclient) restart() error {

	conn := api.connection

	fmt.Fprintf(conn, "%s\n", commandRestart)
	fmt.Fprintf(conn, "node: %d\n", 2)
	fmt.Fprintf(conn, "abort: %d\n", 0)
	fmt.Fprintf(conn, "initialstart: %d\n", 0)
	fmt.Fprintf(conn, "nostart: %d\n", 0)
	fmt.Fprintf(conn, "force: %d\n", 1)
	fmt.Fprintf(conn, "\n")

	buf, err := api.getReplySlowCommand()
	if err != nil {
		return err
	}

	lineno := 1
	scanner := bufio.NewScanner(strings.NewReader(string(buf)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fmt.Printf("[%d]  %s\n", lineno, line)
		lineno++
	}

	return nil
}

func (api *Mgmclient) StopNodes(nodeIds *[]int) (bool, error) {

	conn := api.connection

	nodeListStr := ""
	sep := ""

	for node := 0; node < len(*nodeIds); node++ {
		nodeListStr += fmt.Sprintf("%s%d", sep, (*nodeIds)[node])
		sep = " "
	}

	fmt.Fprintf(conn, "%s\n", "stop v2")
	fmt.Fprintf(conn, "node: %s\n", nodeListStr)
	fmt.Fprintf(conn, "abort: %d\n", 0)
	fmt.Fprintf(conn, "force: %d\n", 0)
	fmt.Fprintf(conn, "\n")

	buf, err := api.getReplySlowCommand()
	if err != nil {
		return false, err
	}

	// [1]  stop reply
	// [2]  result: Ok
	// [3]  stopped: 1
	// [4]  disconnect: 0
	// [5]

	lineno := 1
	disconnect := 0

	scanner := bufio.NewScanner(strings.NewReader(string(buf)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) < 1 {
			lineno++
			continue
		}

		if lineno == 1 {
			if line != "stop reply" {
				s := fmt.Sprintf("Format error in line %d: %s", lineno, line)
				return false, errors.New(s)
			}
			lineno++
			continue
		}

		s := strings.SplitN(line, ":", 2)
		if len(s) != 2 {
			return false, errors.New("Format error (missing result)" + fmt.Sprint(lineno) + " " + line)
		}
		key := s[0]
		val := strings.TrimSpace(s[1])

		switch key {
		case "result":
			if val != "Ok" {
				return false, errors.New(val)
			}
			break
		case "disconnect":
			disc, err := strconv.Atoi(val)
			if err != nil {
				// we ignore error here
			} else {
				disconnect = disc
			}
			break
		}

		//fmt.Printf("[%d]  %s\n", lineno, line)
		lineno++
	}

	return disconnect == 1, nil
}

func (api *Mgmclient) showConfig() error {

	conn := api.connection

	fmt.Fprintf(conn, "show config\n")
	fmt.Fprintf(conn, "\n")

	buf, err := api.getReply()
	if err != nil {
		return err
	}

	lineno := 1
	scanner := bufio.NewScanner(strings.NewReader(string(buf)))
	for scanner.Scan() {
		//line := strings.TrimSpace(scanner.Text())
		//fmt.Printf("[%d]  %s\n", lineno, line)
		lineno++
	}
	return nil
}

func (api *Mgmclient) GetOwnNodeId() (int, error) {

	// get mgmd nodeid

	// get mgmd nodeid reply
	// nodeid:%u

	conn := api.connection

	fmt.Fprintf(conn, "get mgmd nodeid\n")
	fmt.Fprintf(conn, "\n")

	buf, err := api.getReply()
	if err != nil {
		return 0, err
	}

	ownNodeId := 0

	lineno := 1
	scanner := bufio.NewScanner(strings.NewReader(string(buf)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) < 1 {
			continue
		}

		if lineno == 1 {
			if line != "get mgmd nodeid reply" {
				return 0, fmt.Errorf("Format error in management server reply in line %d: %s", lineno, line)
			}
		} else {
			s := strings.SplitN(line, ":", 2)
			if len(s) != 2 {
				return 0, errors.New("Format error " + fmt.Sprint(lineno) + " " + line)
			}
			key := s[0]
			val := strings.TrimSpace(s[1])

			if key != "nodeid" {
				return 0, fmt.Errorf("Format error in management server reply in line %d: %s", lineno, line)
			}

			ownNodeId, err = strconv.Atoi(val)
			if err != nil {
				return 0, errors.New("Format error (unable to extract node count) " + fmt.Sprint(lineno) + " " + line)
			}
		}
		lineno++
	}

	return ownNodeId, nil
}

func (api *Mgmclient) ShowVariables() (*map[string]string, error) {

	conn := api.connection

	fmt.Fprintf(conn, "show variables\n")
	fmt.Fprintf(conn, "\n")

	buf, err := api.getReply()
	if err != nil {
		return nil, err
	}

	variables := make(map[string]string)

	lineno := 1
	scanner := bufio.NewScanner(strings.NewReader(string(buf)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) < 1 {
			continue
		}
		//fmt.Printf("[%d] %s\n", lineno, line)

		if lineno == 1 {
			if line != "show variables reply" {
				s := fmt.Sprintf("Format error in show variables reply (wrong header \"%s\" in line %d)", line, lineno)
				return nil, errors.New(s)
			}
			lineno++
			continue
		}

		s := strings.SplitN(line, ":", 2)
		if len(s) != 2 {
			return nil, errors.New("Format error " + fmt.Sprint(lineno) + " " + line)
		}
		key := s[0]
		val := strings.TrimSpace(s[1])

		variables[key] = val
	}

	return &variables, nil
}

func (api *Mgmclient) GetStatus() (*ClusterStatus, error) {

	conn := api.connection

	fmt.Fprintf(conn, "get status\n")
	fmt.Fprintf(conn, "\n")

	buf, err := api.getReply()
	if err != nil {
		return nil, err
	}

	// a reply usually looks like this:
	// [1]  node status
	// [2]  nodes: 8
	// [3]  node.3.type: NDB

	var nss *ClusterStatus
	lineno := 1
	scanner := bufio.NewScanner(strings.NewReader(string(buf)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) < 1 {
			continue
		}
		//fmt.Printf("[%d]  %s\n", lineno, line)

		// first line is just text "node status"
		if lineno == 1 {
			if line == "node status" {
				lineno++
				continue
			}
			return nil, errors.New("Format error (missing \"node status\" header " + fmt.Sprint(lineno) + " " + line)
		}

		s := strings.SplitN(line, ":", 2)
		if len(s) != 2 {
			return nil, errors.New("Format error " + fmt.Sprint(lineno) + " " + line)
		}
		nodeStr := s[0]
		val := strings.TrimSpace(s[1])

		// read node of nodes entry and create map
		if lineno == 2 {
			if nodeStr != "nodes" {
				return nil, errors.New("Format error (missing keyword nodes) " + fmt.Sprint(lineno) + " " + line)
			}
			nodeCount, err := strconv.Atoi(val)
			if err != nil {
				return nil, errors.New("Format error (unable to extract node count) " + fmt.Sprint(lineno) + " " + line)
			}
			nss = NewClusterStatus(nodeCount)
			lineno++
			continue
		}

		// now split a status line like
		// "node.3.version: 524310"

		s2 := strings.SplitN(nodeStr, ".", 3)
		if len(s2) != 3 {
			return nil, errors.New("Format error (can't parse node status string) " + fmt.Sprint(lineno) + " " + line)
		}

		// node id
		nodeId, err := strconv.Atoi(s2[1])
		// "node.*3*.version: 524310"
		if err != nil {
			return nil, errors.New("Format error (can't extract node id)" + fmt.Sprint(lineno) + " " + line)
		}

		// type of the status value (e.g. softare version, connection status)
		// "node.3.*version*: 524310"
		valType := s2[2]

		nodeStatus := nss.EnsureNode(nodeId)

		// based on the type of value store status away
		switch valType {
		case "type":
			err = nss.SetNodeTypeFromTLA(nodeId, val)
			if err != nil {
				return nil, errors.New("Format error (can't extract node type)" + fmt.Sprint(lineno) + " " + line)
			}
			break

		case "status":
			// status NDB == STARTED or MGM == CONNECTED
			nodeStatus.IsConnected = false
			if nodeStatus.NodeType == MgmNodeTypeID && val == "CONNECTED" {
				nodeStatus.IsConnected = true
			}
			if nodeStatus.NodeType == DataNodeTypeID && val == "STARTED" {
				nodeStatus.IsConnected = true
			}
			break
		case "version":
			valint, err := strconv.Atoi(val)
			if err != nil {
				return nil, errors.New("Format error (can't extract software version) " + fmt.Sprint(lineno) + " " + line)
			}

			if valint == 0 {
				nodeStatus.SoftwareVersion = ""
			} else {
				major := (valint >> 16) & 0xFF
				minor := (valint >> 8) & 0xFF
				build := (valint >> 0) & 0xFF
				nodeStatus.SoftwareVersion = fmt.Sprintf("%d.%d.%d", major, minor, build)
			}

			break

		case "node_group":
			valint, err := strconv.Atoi(val)
			if err != nil {
				return nil, errors.New("Format error (can't extract software version) " + fmt.Sprint(lineno) + " " + line)
			}
			nodeStatus.NodeGroup = valint

			break
		}

		lineno++
	}

	// correct some data based on other data in respective node status
	for _, ns := range *nss {

		// if node is not started then we don't really know its node group reliably
		ng := -1
		if ns.NodeType == DataNodeTypeID && ns.IsConnected {
			ng = ns.NodeGroup
		}
		ns.NodeGroup = ng
	}

	return nss, nil
}

const V2_TYPE_SHIFT = 28
const V2_TYPE_MASK = 15
const V2_KEY_SHIFT = 0
const V2_KEY_MASK = 0x0FFFFFFF

func readUint32(data []byte, offset *int) uint32 {
	v := binary.BigEndian.Uint32(data[*offset : *offset+4])
	*offset += 4
	return v
}

func readString(data []byte, offset *int) string {
	len := int(readUint32(data, offset))
	s := string(data[*offset : *offset+len])
	len = len + ((4 - (len & 3)) & 3)
	*offset += len
	return s
}

func (api *Mgmclient) readEntry(data []byte, offset *int) (uint32, interface{}, error) {

	key := readUint32(data, offset)

	key_type := (key >> V2_TYPE_SHIFT) & V2_TYPE_MASK

	key = (key >> V2_KEY_SHIFT) & V2_KEY_MASK

	switch key_type {
	case IntTypeId:
		{
			val := readUint32(data, offset)
			//fmt.Printf("key: %d = %d\n", key, val)
			return key, val, nil
		}
	case Int64TypeId:
		{
			high := readUint32(data, offset)
			low := readUint32(data, offset)
			val := (uint64(high) << 32) + uint64(low)
			//fmt.Printf("key: %d = %d\n", key, val)
			return key, val, nil
		}
	case StringTypeId:
		{
			val := readString(data, offset)
			//fmt.Printf("key: %d = %s\n", key, val)
			return key, val, nil
		}
	default:
	}

	return key, nil, nil
}

func (api *Mgmclient) GetConfig() (*ConfigSection, error) {
	return api.GetConfigFromNode(0)
}

func (api *Mgmclient) GetConfigFromNode(fromNodeId int) (*ConfigSection, error) {
	conn := api.connection
	// send to server
	//		fmt.Fprintln(conn, "get status")
	//		fmt.Fprintln(conn, "")

	fmt.Fprintf(conn, "get config_v2\n")

	// TODO: add real version here
	fmt.Fprintf(conn, "%s: %d\n", "version", 524311)

	// TODO: clarify use of nodetype
	fmt.Fprintf(conn, "%s: %d\n", "nodetype", -1)

	// TODO: which node is this?
	fmt.Fprintf(conn, "%s: %d\n", "node", 1)

	if fromNodeId > 0 {
		fmt.Fprintf(conn, "%s: %d\n", "from_node", fromNodeId)
	}
	fmt.Fprintf(conn, "\n")

	buf, err := api.getReply()
	if err != nil {
		return nil, err
	}

	reply := []string{
		"",
		"get config reply",
		"result",
		"Content-Length",
		"Content-Type",
		"Content-Transfer-Encoding",
	}

	lineno := 1
	scanner := bufio.NewScanner(strings.NewReader(string(buf)))
	base64str := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if lineno < len(reply) {

		} else {
			base64str += line
		}
		//fmt.Printf("[%d] %s\n", lineno, line)
		lineno++
	}

	data, err := base64.StdEncoding.DecodeString(base64str)
	if err != nil {
		return nil, err
	}
	//fmt.Printf("%q\n", data)

	//magic := string(data[0:8])
	/*
	* Header section (7 words)
	*  1. Total length in words of configuration binary
	*  2. Configuration binary version (this is version 2)
	*  3. Number of default sections in configuration binary
	*     - Data node defaults
	*     - API node defaults
	*     - MGM server node defaults
	*     - TCP communication defaults
	*     - SHM communication defaults
	*     So always 5 in this version
	*  4. Number of data nodes
	*  5. Number of API nodes
	*  6. Number of MGM server nodes
	*  7. Number of communication sections
	 */
	header := data[8 : 8+4*7]

	//fmt.Printf("magic: %s\n", magic)
	//fmt.Printf("header len: %d\n", len(header))

	offset := int(0)
	readUint32(header, &offset) //totalLen := readUint32(header, &offset)
	readUint32(header, &offset) //version := readUint32(header, &offset)
	noOfDefaults := int(binary.BigEndian.Uint32(header[8:12]))
	noOfDN := binary.BigEndian.Uint32(header[12:16])
	noOfAPI := binary.BigEndian.Uint32(header[16:20])
	noOfMGM := binary.BigEndian.Uint32(header[20:24])
	noOfTCP := binary.BigEndian.Uint32(header[24:28])

	//fmt.Printf("length: %d, version: %d, default sections: %d, DN: %d, API: %d, MGM: %d, TCP: %d\n",
	//	totalLen, version, noOfDefaults, noOfDN, noOfAPI, noOfMGM, noOfTCP)

	offset = 8 + 28

	for ds := 0; ds < noOfDefaults; ds++ {
		api.readSection(data, &offset)
	}

	// system
	s := api.readSection(data, &offset)

	// nodes
	for n := 0; n < int(noOfDN+noOfMGM+noOfAPI); n++ {
		api.readSection(data, &offset)
	}

	// comm
	for n := 0; n < int(noOfTCP); n++ {
		api.readSection(data, &offset)
	}

	return s, nil
}

func (api *Mgmclient) GetConfigVersion() int {
	cs, err := api.GetConfig()
	if err != nil {
		return -1
	}
	if val, ok := (*cs)[uint32(CFG_SYS_CONFIG_GENERATION)]; ok {
		if v, ok := val.(uint32); ok {
			return int(v)
		}
		if v, ok := val.(uint64); ok {
			return int(v)
		}
	}
	return -1
}

func (api *Mgmclient) readSection(data []byte, offset *int) *ConfigSection {

	// section header data nodes
	readUint32(data, offset) //len := readUint32(data, offset)
	noEntries := int(readUint32(data, offset))
	readUint32(data, offset) //nodeType := readUint32(data, offset)

	cs := make(ConfigSection, noEntries)

	//fmt.Printf("len: %d, no entries: %d, type: %d\n", len, noEntries, nodeType)

	for e := 0; e < noEntries; e++ {
		key, val, _ := api.readEntry(data, offset)
		cs[key] = val
	}
	//fmt.Printf("offset: %d\n", *offset)

	return &cs
}

func (api *Mgmclient) getReply() ([]byte, error) {
	return api.getReplyWithTimeout(defaultTimeoutInSeconds)
}

func (api *Mgmclient) getReplySlowCommand() ([]byte, error) {
	return api.getReplyWithTimeout(5 * 60)
}

func (api *Mgmclient) getReplyWithTimeout(timoutInSeconds time.Duration) ([]byte, error) {
	// wait for reply
	var tmp []byte
	var buf []byte
	tmp = make([]byte, 64)

	err := api.connection.SetReadDeadline(time.Now().Add(timoutInSeconds * time.Second))
	if err != nil {
		log.Println("SetReadDeadline failed:", err)
		// do something else, for example create new conn
		return nil, err
	}

	retries := 0
	var reterror error
	for {
		var n int
		if retries > 0 {
			klog.Infof("read retry # %d", retries)
		}
		n, err = api.connection.Read(tmp)
		if err != nil {
			/* there are 3 types of errors that we distinguish:
				- timeout, other read errors and EOF
			 EOF occurs also on retry of the other 2 types
			 thus we store away only the other two types in order to report the actual error
			*/
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				reterror = netErr
				// time out
			} else {
				if err != io.EOF {
					reterror = err
				}
			}
			if retries > maxReadRetries {
				if reterror == nil {
					reterror = err
				}
				break
			}
			retries++
		} else {
			buf = append(buf, tmp[:n]...)
			if n < 64 {
				break
			}
		}
	}
	return buf, reterror
}
