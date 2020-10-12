// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package helpers

import (
	"fmt"
	"os"
	"testing"
)

func TestReadInifile(t *testing.T) {

	f, err := os.Create("test.txt")
	if err != nil {
		fmt.Println(err)
		return
	}

	testini := `
	[ndbd default]
	NoOfReplicas=2
	DataMemory=80M
	ServerPort=2202
	StartPartialTimeout=15000
	StartPartitionedTimeout=0
	
	[tcp default]
	AllowUnresolvedHostnames=1
	
	[ndb_mgmd]
	NodeId=0
	Hostname=example-ndb-0.example-ndb.svc.default-namespace.com
	DataDir=/var/lib/ndb
	
	[ndbd]
	NodeId=1
	Hostname=example-ndb-0.example-ndb.svc.default-namespace.com
	DataDir=/var/lib/ndb
	ServerPort=1186
	
	[mysqld]
	NodeId=1
	Hostname=example-ndb-0.example-ndb.svc.default-namespace.com
	
	[mysqld]
	`

	l, err := f.WriteString(testini)
	if err != nil {
		t.Error(err)
		f.Close()
		return
	}

	if l == 0 {
		f.Close()
		t.Fail()
		return
	}
	f.Close()

	c, err := parseFile("test.txt")

	if err != nil {
		t.Error(err)
		f.Close()
		return
	}

	if c == nil {
		t.Fail()
		return
	}

	t.Log("Iterating")
	for s, grp := range c.Groups {
		for _, sec := range grp {
			t.Log("[" + s + "] ")
			for key, val := range sec {
				t.Log(key + ": " + val)
			}
		}
	}
}
