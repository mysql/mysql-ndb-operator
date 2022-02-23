// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbconfig

import (
	"testing"

	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
)

func errorIfNotEqual(t *testing.T, expected, actual int32, desc string) {
	t.Helper()
	if expected != actual {
		t.Errorf("Actual '%s' value(%d) didn't match the expected value(%d).", desc, actual, expected)
	}
}

func Test_NewConfigSummary(t *testing.T) {

	// just testing actual extraction of ConfigHash - not ini-file reading
	testini := `
	;
	; this is a header section
	[system]
	ConfigGenerationNumber=4711
    [ndbd default]
	NoOfReplicas=2
    DataMemory=42
    IndexMemory=128
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

	cs, err := NewConfigSummary(map[string]string{
		constants.ConfigIniKey:         testini,
		constants.NdbClusterGeneration: "3",
		constants.NumOfMySQLServers:    "3",
		constants.FreeApiSlots:         "10",
	})

	if err != nil {
		t.Errorf("NewConfigSummary failed : %s", err)
	}

	errorIfNotEqual(t, 3, int32(cs.NdbClusterGeneration), "cs.MySQLClusterConfigNeedsUpdate")
	errorIfNotEqual(t, 4711, cs.MySQLClusterConfigVersion, "cs.MySQLClusterConfigVersion")
	errorIfNotEqual(t, 2, cs.RedundancyLevel, " cs.RedundancyLevel")
	errorIfNotEqual(t, 2, cs.NumOfManagementNodes, "cs.NumOfManagementNodes")
	errorIfNotEqual(t, 2, cs.NumOfDataNodes, "cs.NumOfDataNodes")
	errorIfNotEqual(t, 6, cs.TotalNumOfApiSlots, "cs.TotalNumOfApiSlots")
	errorIfNotEqual(t, 3, cs.NumOfMySQLServers, "cs.NumOfMySQLServers")
	errorIfNotEqual(t, 10, cs.NumOfFreeApiSlots, "cs.NumOfFreeApiSlots")
	errorIfNotEqual(t, 42, parseInt32(cs.getDefaultNdbdConfigValue("DataMemory")), "DataMemory")
}

func Test_GetConfigString(t *testing.T) {
	ndb := testutils.NewTestNdb("default", "example-ndb", 2)
	ndb.Spec.DataMemory = "80M"
	ndb.Spec.FreeAPISlots = 3
	configString, err := GetConfigString(ndb, nil)
	if err != nil {
		t.Errorf("Failed to generate config string from Ndb : %s", err)
	}

	expectedConfigString := `# Auto generated config.ini - DO NOT EDIT

[system]
ConfigGenerationNumber=1
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

[api]
NodeId=150

`
	if configString != expectedConfigString {
		t.Error("The generated config string does not match the expected value")
		t.Errorf("Expected :\n%s\n", expectedConfigString)
		t.Errorf("Generated :\n%s\n", configString)
	}

}
