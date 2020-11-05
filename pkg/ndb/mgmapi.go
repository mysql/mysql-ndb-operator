package ndb

import (
	"bufio"
	"encoding/base64"
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

	return nil
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
