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
	;
	; this is a header section
	;
	; ConfigHash=asdasdlkajhhnxh=?   
	;               notice this ^

	; should not create 2nd header with just empty line

	[ndbd default]
	NoOfReplicas=2
	DataMemory=80M
	ServerPort=2202
	StartPartialTimeout=15000
	StartPartitionedTimeout=0
	
	[tcp default]
	AllowUnresolvedHostnames=1
	
	# more comments to be ignored

	[ndb_mgmd]
	NodeId=0
	Hostname=example-ndb-0.example-ndb.svc.default-namespace.com
	DataDir=/var/lib/ndb
	
	# key=value
	# comment with key value pair here should be ignored
	[ndbd]
	NodeId=1
	Hostname=example-ndb-0.example-ndb.svc.default-namespace.com
	DataDir=/var/lib/ndb
	ServerPort=1186
	
	[mysqld]
	NodeId=1
	Hostname=example-ndb-0.example-ndb.svc.default-namespace.com
	
	[mysqld]`

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

	c, err := ParseFile("test.txt")

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
			t.Log("[" + s + "]")
			for key, val := range sec {
				t.Log(key + ": " + val)
			}
		}
	}

	t.Fail()
}

func Test_GetNumberOfSections(t *testing.T) {

	// just testing actual extraction of ConfigHash - not ini-file reading
	testini := `
	[ndbd]
	[ndbd]
	[ndbd]
	[ndbd]
	[mgmd]
	[mgmd]
	`

	config, err := ParseString(testini)
	if err != nil {
		t.Errorf("Parsing of config failed: %s", err)
	}

	secs := GetNumberOfSectionsInSectionGroup(config, "ndbd")
	if secs != 4 {
		t.Fail()
	}

	secs = GetNumberOfSectionsInSectionGroup(config, "mysqld")
	if secs != 0 {
		t.Fail()
	}
}
