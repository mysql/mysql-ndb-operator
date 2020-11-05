// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndb

import (
	"testing"
	"time"
)

func TestGetStatus(t *testing.T) {
	api := &mgmclient{}

	err := api.connect()
	if err != nil {
		t.Errorf("Connection failed: %s", err)
		return
	}
	defer api.disconnect()

	for {
		err = api.getStatus()
		if err != nil {
			t.Errorf("get status failed: %s", err)
			return
		}
		time.Sleep(1 * time.Second)
	}
}

func TestRestart(t *testing.T) {
	api := &mgmclient{}

	err := api.connect()
	if err != nil {
		t.Errorf("Connection failed: %s", err)
		return
	}
	defer api.disconnect()

	err = api.restart()
	if err != nil {
		t.Errorf("restart failed : %s", err)
		return
	}
}
