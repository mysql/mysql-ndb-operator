// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import (
	"bufio"
	"net"
	"testing"
)

// fakeMgmServer is used to test mgmClientImpl methods.
// A test can start this fake Mgm Server, with a set of
// replies the server needs to send back and then use a
// Management Client connected to this fake server to
// read and parse those replies.
type fakeMgmServer struct {
	connection net.Conn
	t          *testing.T
}

// Returns a new fakeMgmServer and a connected mgmClientImpl
func newFakeMgmServerAndClient(t *testing.T) (*fakeMgmServer, *mgmClientImpl) {
	// Create a server and a client connected via pipe
	server, client := net.Pipe()
	// Return the fake server and the client
	return &fakeMgmServer{
			connection: server,
			t:          t,
		}, &mgmClientImpl{
			connection: client,
		}
}

func (fmc *fakeMgmServer) disconnect() {
	if fmc.connection != nil {
		_ = fmc.connection.Close()
	}
}

// run starts the fake mgmd and then replies to the
// client with the given data, when a request is sent.
func (fmc *fakeMgmServer) run(replies ...[]byte) {
	// Emulate the fake Management Server in a go routine
	go func() {
		// Consume the command sent by the client
		scanner := bufio.NewScanner(fmc.connection)
		text := "nil"
		// Command ends when an empty line is read
		for text != "" && scanner.Scan() {
			text = scanner.Text()
		}

		if err := scanner.Err(); err != nil {
			fmc.t.Errorf("Fake Mgm Server failed to read request from the client : %s", err)
			return
		}

		// Send the replies back to client
		for _, reply := range replies {
			if _, err := fmc.connection.Write(reply); err != nil {
				fmc.t.Errorf("Fake Mgm Server failed to send reply to the client : %s", err)
				return
			}
		}

		// Send empty line to mark end of reply
		if _, err := fmc.connection.Write([]byte("\n\n")); err != nil {
			fmc.t.Errorf("Fake Mgm Server failed to send reply to the client : %s", err)
			return
		}
	}()
}
