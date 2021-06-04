// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"fmt"
	"testing"

	"github.com/mysql/ndb-operator/pkg/helpers"
)

func Test_NewResourceContextFromConfiguration(t *testing.T) {

	// just testing actual extraction of ConfigHash - not ini-file reading
	testini := `
	;
	; this is a header section
	;
	;ConfigHash=asdasdlkajhhnxh=?   
	;               notice this ^
	[system]
	ConfigGenerationNumber=4711
	`

	rc, err := NewResourceContextFromConfiguration(testini)

	if err != nil {
		t.Errorf("extracting hash or generation failed : %s", err)
	}

	if rc.ConfigHash != "asdasdlkajhhnxh=?" {
		t.Fail()
	}
	if rc.ConfigGeneration != 4711 {
		t.Errorf("Wrong generation :" + fmt.Sprint(rc.ConfigGeneration))
	}
}

func Test_NewResourceContextFromConfiguration2(t *testing.T) {

	// just testing actual extraction of ConfigHash - not ini-file reading
	testini := `
	[ndbd default]
	NoOfReplicas=2
	[ndbd]
	[ndbd]
	[ndb_mgmd]
	[ndb_mgmd]
	`

	rc, err := NewResourceContextFromConfiguration(testini)

	if err != nil {
		t.Errorf("extracting hash or generation failed : %s", err)
	}

	if rc.ManagementNodeCount != 2 {
		t.Fail()
	}
	if rc.ReduncancyLevel != 2 {
		t.Fail()
	}
	if rc.ConfiguredNodeGroupCount != 1 {
		t.Fail()
	}
}

func Test_GetConfigString(t *testing.T) {
	ndb := helpers.NewTestNdb("default", "example-ndb", 2)
	configString, err := GetConfigString(ndb)
	if err != nil {
		t.Errorf("Failed to generate config string from Ndb : %s", err)
	}

	expectedConfigString := `# auto generated config.ini - do not edit
#
# ConfigHash=/MYzVPzS/vQRVtW4+ZIFMg==

[system]
ConfigGenerationNumber=0
Name=example-ndb

[ndbd default]
NoOfReplicas=2
DataMemory=80M
# Use a fixed ServerPort for all data nodes
ServerPort=1186

[tcp default]
AllowUnresolvedHostnames=1

[ndb_mgmd]
NodeId=1
Hostname=example-ndb-mgmd-0.example-ndb-mgmd.default.svc.cluster.local
DataDir=/var/lib/ndb

[ndb_mgmd]
NodeId=2
Hostname=example-ndb-mgmd-1.example-ndb-mgmd.default.svc.cluster.local
DataDir=/var/lib/ndb

[ndbd]
NodeId=3
Hostname=example-ndb-ndbd-0.example-ndb-ndbd.default.svc.cluster.local
DataDir=/var/lib/ndb

[ndbd]
NodeId=4
Hostname=example-ndb-ndbd-1.example-ndb-ndbd.default.svc.cluster.local
DataDir=/var/lib/ndb

[mysqld]
NodeId=145

[mysqld]
NodeId=146

[mysqld]
NodeId=147

`
	if configString != expectedConfigString {
		t.Error("The generated config string does not match the expected value")
		t.Errorf("Expected :\n%s\n", expectedConfigString)
		t.Errorf("Generated :\n%s\n", configString)
	}

}
