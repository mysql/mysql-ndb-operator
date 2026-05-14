// Copyright (c) 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mysqlclient

import (
	"bytes"
	"database/sql"
	"errors"
	"strings"
	"testing"

	klog "k8s.io/klog/v2"
)

var errCreateRootUser = errors.New("synthetic create user failure")

func TestCreateRootUserIfNotExistDoesNotLogPasswordOnCreateUserError(t *testing.T) {
	klogState := klog.CaptureState()
	defer klogState.Restore()

	var logOutput bytes.Buffer
	klog.SetOutput(&logOutput)
	klog.LogToStderr(false)

	rootPassword := "root-secret-password"
	executor := &rootUserCreateFailExecutor{}
	err := createRootUser(executor, "%", rootPassword)
	if !errors.Is(err, errCreateRootUser) {
		t.Fatalf("Expected CREATE USER error %q, got %v", errCreateRootUser, err)
	}
	if !strings.Contains(executor.query, rootPassword) {
		t.Fatalf("Expected CREATE USER query to contain the root password")
	}
	klog.Flush()

	logged := logOutput.String()
	if strings.Contains(logged, rootPassword) {
		t.Fatalf("Root password was logged: %s", logged)
	}
	if strings.Contains(logged, "identified by") {
		t.Fatalf("CREATE USER statement was logged: %s", logged)
	}
	if !strings.Contains(logged, "CREATE USER 'root'@'%' failed") {
		t.Fatalf("Expected safe CREATE USER error log, got: %s", logged)
	}
	if !strings.Contains(logged, errCreateRootUser.Error()) {
		t.Fatalf("Expected error detail in log, got: %s", logged)
	}
}

type rootUserCreateFailExecutor struct {
	query string
}

func (e *rootUserCreateFailExecutor) Exec(query string, _ ...any) (sql.Result, error) {
	e.query = query
	return nil, errCreateRootUser
}
