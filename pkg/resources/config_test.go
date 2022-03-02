// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"regexp"
	"testing"

	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
)

func errorIfNotEqual(t *testing.T, expected, actual uint32, desc string) {
	t.Helper()
	if expected != actual {
		t.Errorf("Actual '%s' value(%d) didn't match the expected value(%d).", desc, actual, expected)
	}
}

func Test_NewResourceContextFromConfiguration(t *testing.T) {

	// just testing actual extraction of ConfigHash - not ini-file reading
	testini := `
	;
	; this is a header section
	;
	;ConfigHash=asdasdlkajhhnxh=?   
	;               notice this ^
	;NumOfMySQLServers=3
	[system]
	ConfigGenerationNumber=4711
    [ndbd default]
	NoOfReplicas=2
	[ndbd]
	[ndbd]
	[ndb_mgmd]
	[ndb_mgmd]
    [api]
    [api]
    [api]
    [api]
    [api]
    [api]
	`

	rc, err := NewResourceContextFromConfiguration(testini)

	if err != nil {
		t.Errorf("NewResourceContextFromConfiguration failed : %s", err)
	}

	if rc.ConfigHash != "asdasdlkajhhnxh=?" {
		t.Errorf("Actual 'rc.ConfigHash' value(%s) didn't match the expected value(asdasdlkajhhnxh=?).", rc.ConfigHash)
	}
	errorIfNotEqual(t, 4711, rc.ConfigGeneration, "rc.ConfigGeneration")
	errorIfNotEqual(t, 2, rc.RedundancyLevel, " rc.RedundancyLevel")
	errorIfNotEqual(t, 2, rc.NumOfManagementNodes, "rc.NumOfManagementNodes")
	errorIfNotEqual(t, 2, rc.NumOfDataNodes, "rc.NumOfDataNodes")
	errorIfNotEqual(t, 6, rc.NumOfApiSlots, "rc.NumOfApiSlots")
	errorIfNotEqual(t, 3, rc.NumOfMySQLServers, "rc.NumOfMySQLServers")
}

func Test_GetConfigString(t *testing.T) {
	ndb := testutils.NewTestNdb("default", "example-ndb", 2)
	ndb.Spec.DataMemory = "80M"
	configString, err := GetConfigString(ndb, nil)
	if err != nil {
		t.Errorf("Failed to generate config string from Ndb : %s", err)
	}

	expectedConfigString := `# auto generated config.ini - do not edit
#
# ConfigHash=######
# NumOfMySQLServers=2

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

[api]
NodeId=145

[api]
NodeId=146

[api]
NodeId=147

[api]
NodeId=148

[api]
NodeId=149

`
	// replace the config hash
	re := regexp.MustCompile(`ConfigHash=.*`)
	configString = re.ReplaceAllString(configString, "ConfigHash=######")

	if configString != expectedConfigString {
		t.Error("The generated config string does not match the expected value")
		t.Errorf("Expected :\n%s\n", expectedConfigString)
		t.Errorf("Generated :\n%s\n", configString)
	}

}

func Test_GetConfigString_ApiFreeSlots(t *testing.T) {
	ndb := testutils.NewTestNdb("default", "example-ndb-2", 1)
	ndb.Spec.DataMemory = "80M"
	ndb.Spec.ApiFreeSlots = 10
	configString, err := GetConfigString(ndb, nil)
	if err != nil {
		t.Errorf("Failed to generate config string from Ndb : %s", err)
	}

	expectedConfigString := `# auto generated config.ini - do not edit
#
# ConfigHash=######
# NumOfMySQLServers=1

[system]
ConfigGenerationNumber=0
Name=example-ndb-2

[ndbd default]
NoOfReplicas=2
DataMemory=80M
# Use a fixed ServerPort for all data nodes
ServerPort=1186

[tcp default]
AllowUnresolvedHostnames=1

[ndb_mgmd]
NodeId=1
Hostname=example-ndb-2-mgmd-0.example-ndb-2-mgmd.default.svc.cluster.local
DataDir=/var/lib/ndb

[ndb_mgmd]
NodeId=2
Hostname=example-ndb-2-mgmd-1.example-ndb-2-mgmd.default.svc.cluster.local
DataDir=/var/lib/ndb

[ndbd]
NodeId=3
Hostname=example-ndb-2-ndbd-0.example-ndb-2-ndbd.default.svc.cluster.local
DataDir=/var/lib/ndb

[api]
NodeId=145

[api]
NodeId=146

[api]
NodeId=147

[api]
NodeId=148

[api]
NodeId=149

[api]
NodeId=150

[api]
NodeId=151

[api]
NodeId=152

[api]
NodeId=153

[api]
NodeId=154

[api]
NodeId=155

`
	// replace the config hash
	re := regexp.MustCompile(`ConfigHash=.*`)
	configString = re.ReplaceAllString(configString, "ConfigHash=######")

	if configString != expectedConfigString {
		t.Error("The generated config string does not match the expected value")
		t.Errorf("Expected :\n%s\n", expectedConfigString)
		t.Errorf("Generated :\n%s\n", configString)
	}

}
