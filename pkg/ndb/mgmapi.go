package ndb

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"k8s.io/klog"
)

const maxReadRetries = 6
const defaultTimeoutInSeconds = 5

const commandRestart = "restart node v2"

type mgmclient struct {
	connection net.Conn
}

func (api *mgmclient) disconnect() {
	if api.connection != nil {
		api.connection.Close()
	}
}

func (api *mgmclient) connect() error {

	// connect to server
	var err error
	api.connection, err = net.Dial("tcp", "127.0.0.1:1186")
	if err != nil {
		log.Println("Connecting to management server failed:", err)
		// do something else, for example create new conn
		return err
	}

	return nil
}

func (api *mgmclient) restart() error {

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

func (api *mgmclient) showConfig() error {

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
		line := strings.TrimSpace(scanner.Text())
		fmt.Printf("[%d]  %s\n", lineno, line)
		lineno++
	}
	return nil
}

func (api *mgmclient) getStatus() error {

	conn := api.connection

	fmt.Fprintf(conn, "get status\n")
	fmt.Fprintf(conn, "\n")

	buf, err := api.getReply()
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

const InvalidTypeId = 0
const IntTypeId = 1
const StringTypeId = 2
const SectionTypeId = 3
const Int64TypeId = 4

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

func (api *mgmclient) readEntry(data []byte, offset *int) error {

	key := readUint32(data, offset)

	key_type := (key >> V2_TYPE_SHIFT) & V2_TYPE_MASK

	key = (key >> V2_KEY_SHIFT) & V2_KEY_MASK

	switch key_type {
	case IntTypeId:
		{
			val := readUint32(data, offset)
			fmt.Printf("key: %d = %d\n", key, val)
			break
		}
	case Int64TypeId:
		{
			high := readUint32(data, offset)
			low := readUint32(data, offset)
			val := (uint64(high) << 32) + uint64(low)
			fmt.Printf("key: %d = %d\n", key, val)
			break
		}
	case StringTypeId:
		{
			val := readString(data, offset)
			fmt.Printf("key: %d = %s\n", key, val)
			break
		}
	default:
		{
			return nil
		}
	}

	return nil
}

func (api *mgmclient) getConfig() error {

	conn := api.connection
	// send to server
	//		fmt.Fprintln(conn, "get status")
	//		fmt.Fprintln(conn, "")

	fmt.Fprintf(conn, "get config_v2\n")
	fmt.Fprintf(conn, "%s: %d\n", "version", 524311)
	fmt.Fprintf(conn, "%s: %d\n", "nodetype", -1)
	fmt.Fprintf(conn, "%s: %d\n", "node", 1)
	fmt.Fprintf(conn, "\n")

	buf, err := api.getReply()
	if err != nil {
		return err
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
		fmt.Printf("[%d] %s\n", lineno, line)
		lineno++
	}

	data, err := base64.StdEncoding.DecodeString(base64str)
	if err != nil {
		return err
	}
	fmt.Printf("%q\n", data)

	magic := string(data[0:8])
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

	fmt.Printf("magic: %s\n", magic)
	fmt.Printf("header len: %d\n", len(header))

	offset := int(0)
	totalLen := readUint32(header, &offset)
	version := readUint32(header, &offset)
	noOfDefaults := int(binary.BigEndian.Uint32(header[8:12]))
	noOfDN := binary.BigEndian.Uint32(header[12:16])
	noOfAPI := binary.BigEndian.Uint32(header[16:20])
	noOfMGM := binary.BigEndian.Uint32(header[20:24])
	noOfTCP := binary.BigEndian.Uint32(header[24:28])

	fmt.Printf("length: %d, version: %d, default sections: %d, DN: %d, API: %d, MGM: %d, TCP: %d\n",
		totalLen, version, noOfDefaults, noOfDN, noOfAPI, noOfMGM, noOfTCP)

	offset = 8 + 28

	for ds := 0; ds < noOfDefaults; ds++ {
		api.readSection(data, &offset)
	}

	// system
	api.readSection(data, &offset)

	// nodes
	for n := 0; n < int(noOfDN+noOfMGM+noOfAPI); n++ {
		api.readSection(data, &offset)
	}

	// comm
	for n := 0; n < int(noOfTCP); n++ {
		api.readSection(data, &offset)
	}

	return nil
}

func (api *mgmclient) readSection(data []byte, offset *int) {

	// section header data nodes
	len := readUint32(data, offset)
	noEntries := int(readUint32(data, offset))
	nodeType := readUint32(data, offset)

	fmt.Printf("len: %d, no entries: %d, type: %d\n", len, noEntries, nodeType)

	for e := 0; e < noEntries; e++ {
		api.readEntry(data, offset)
	}
	fmt.Printf("offset: %d\n", *offset)
}

func (api *mgmclient) getReply() ([]byte, error) {
	return api.getReplyWithTimeout(defaultTimeoutInSeconds)
}

func (api *mgmclient) getReplySlowCommand() ([]byte, error) {
	return api.getReplyWithTimeout(5 * 60)
}

func (api *mgmclient) getReplyWithTimeout(timoutInSeconds time.Duration) ([]byte, error) {
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
