// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbtest

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"

	klog "k8s.io/klog/v2"
)

// cmdPro is a wrapper around exec.Cmd
type cmdPro struct {
	cmd       *exec.Cmd
	data      string
	ctxCancel context.CancelFunc
}

// newCmdWithTimeout defines a new Cmd with the given timeout
func newCmdWithTimeout(name, data string, timeout time.Duration, args ...string) *cmdPro {

	if timeout == 0 {
		panic("cannot set 0 as the timeout for the " + name + " command")
	}

	// Create a context with given timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Build the command
	cmd := exec.CommandContext(ctx, name, args...)

	// Add the data to the kubectl command stdin
	if data != "" {
		cmd.Stdin = strings.NewReader(data)
	}

	return &cmdPro{
		cmd:       cmd,
		data:      data,
		ctxCancel: cancel,
	}
}

// run the given command with some additional logging.
// It returns the output of the command after execution.
func (cp *cmdPro) run() (stdout, stderr string, err error) {
	// Cleanup context resources before returning
	defer cp.ctxCancel()

	// Redirect stdout/in to a buffer
	cmd := cp.cmd
	var stdoutBytes, stderrBytes bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdoutBytes, &stderrBytes

	// Run the command and return the error
	klog.Infof("Running '%s %s'", cmd.Path, strings.Join(cmd.Args[1:], " "))
	if err = cmd.Run(); err != nil {
		// Command failed. Log the command input and error.
		if cp.data != "" {
			klog.Infof("Command input : \n%s", cp.data)
		}
		klog.Errorf("Command %q failed : %v", cmd.Args[0], err)
	}

	// Send the stdout/stderr to klog
	if stdoutBytes.Len() > 0 {
		klog.Infof("Command stdout : \n%s", stdoutBytes.String())
	}
	if stderrBytes.Len() > 0 {
		klog.Infof("Command stderr : \n%s", stderrBytes.String())
	}

	return stdoutBytes.String(), stderrBytes.String(), err
}
