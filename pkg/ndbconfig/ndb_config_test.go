// Copyright (c) 2020, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbconfig

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
)

func errorIfNotEqual(t *testing.T, expected, actual int32, desc string) {
	t.Helper()
	if expected != actual {
		t.Errorf("Actual '%s' value(%d) didn't match the expected value(%d).", desc, actual, expected)
	}
}

func errorIfNotEqualBool(t *testing.T, expected, actual bool, desc string) {
	t.Helper()
	if expected != actual {
		t.Errorf("Actual '%s' value(%v) didn't match the expected value(%v).", desc, actual, expected)
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
    [mysqld]
    [mysqld]
    [mysqld]
    [api]
    [api]
    [api]
    [api]
    [api]
    [api]
	`

	cs, err := NewConfigSummary(map[string]string{
		constants.ConfigIniKey:           testini,
		constants.NdbClusterGeneration:   "3",
		constants.NumOfMySQLServers:      "3",
		constants.ManagementLoadBalancer: "false",
		constants.MySQLLoadBalancer:      "true",
		constants.DataNodeInitialRestart: "false",
	})

	if err != nil {
		t.Errorf("NewConfigSummary failed : %s", err)
	}

	errorIfNotEqual(t, 3, int32(cs.NdbClusterGeneration), "cs.MySQLClusterConfigNeedsUpdate")
	errorIfNotEqual(t, 4711, cs.MySQLClusterConfigVersion, "cs.MySQLClusterConfigVersion")
	errorIfNotEqual(t, 2, cs.RedundancyLevel, " cs.RedundancyLevel")
	errorIfNotEqual(t, 2, cs.NumOfManagementNodes, "cs.NumOfManagementNodes")
	errorIfNotEqual(t, 2, cs.NumOfDataNodes, "cs.NumOfDataNodes")
	errorIfNotEqual(t, 3, cs.NumOfMySQLServers, "cs.NumOfMySQLServers")
	errorIfNotEqual(t, 3, cs.NumOfMySQLServerSlots, "cs.NumOfMySQLServers")
	errorIfNotEqual(t, 6, cs.NumOfFreeApiSlots, "cs.NumOfFreeApiSlots")
	dataMemory, _ := cs.defaultNdbdSection.GetValue("DataMemory")
	errorIfNotEqual(t, 42, parseInt32(dataMemory), "DataMemory")
	errorIfNotEqualBool(t, false, cs.ManagementLoadBalancer, "cs.ManagementLoadBalancer")
	errorIfNotEqualBool(t, true, cs.MySQLLoadBalancer, "cs.MySQLLoadBalancer")
}

func Test_GetConfigString(t *testing.T) {

	ndb := testutils.NewTestNdb("default", "example-ndb", 2)
	ndb.Spec.FreeAPISlots = 3
	ndb.Spec.MysqlNode.MaxNodeCount = 5
	ndb.Spec.MysqlNode.ConnectionPoolSize = 2
	getIntStrPtr := func(obj intstr.IntOrString) *intstr.IntOrString {
		return &obj
	}
	ndb.Spec.DataNode.Config = map[string]*intstr.IntOrString{
		"DataMemory":        getIntStrPtr(intstr.FromString("80M")),
		"MaxNoOfAttributes": getIntStrPtr(intstr.FromInt(2048)),
		"MaxNoOfTriggers":   getIntStrPtr(intstr.FromInt(10000)),
		"ThreadConfig": getIntStrPtr(
			intstr.FromString("ldm={count=2,cpubind=1,2},main={cpubind=12},rep={cpubind=11}")),
	}
	ndb.Spec.ManagementNode.Config = map[string]*intstr.IntOrString{
		"ExtraSendBufferMemory": getIntStrPtr(intstr.FromString("30M")),
	}
	configString, err := GetConfigString(ndb, nil)
	if err != nil {
		t.Errorf("Failed to generate config string from Ndb : %s", err)
	}

	expectedConfigString := `# Auto generated config.ini - DO NOT EDIT

[system]
ConfigGenerationNumber=1
Name=example-ndb

[ndb_mgmd default]
ExtraSendBufferMemory=30M

[ndbd default]
NoOfReplicas=2
# Use a fixed ServerPort for all data nodes
ServerPort=1186
DataMemory=80M
MaxNoOfAttributes=2048
MaxNoOfTriggers=10000
ThreadConfig=ldm={count=2,cpubind=1,2},main={cpubind=12},rep={cpubind=11}

[tcp default]
AllowUnresolvedHostnames=1

[ndb_mgmd]
NodeId=1
Hostname=example-ndb-mgmd-0.example-ndb-mgmd.default
DataDir=/var/lib/ndb/data

[ndb_mgmd]
NodeId=2
Hostname=example-ndb-mgmd-1.example-ndb-mgmd.default
DataDir=/var/lib/ndb/data

[ndbd]
NodeId=3
Hostname=example-ndb-ndbmtd-0.example-ndb-ndbmtd.default
DataDir=/var/lib/ndb/data

[ndbd]
NodeId=4
Hostname=example-ndb-ndbmtd-1.example-ndb-ndbmtd.default
DataDir=/var/lib/ndb/data

# Dedicated API section to be used by NDB Operator
[api]
NodeId=147
Dedicated=1

# MySQLD sections to be used exclusively by MySQL Servers
[mysqld]
NodeId=148
Hostname=example-ndb-mysqld-0.example-ndb-mysqld.default

[mysqld]
NodeId=149
Hostname=example-ndb-mysqld-0.example-ndb-mysqld.default

[mysqld]
NodeId=150
Hostname=example-ndb-mysqld-1.example-ndb-mysqld.default

[mysqld]
NodeId=151
Hostname=example-ndb-mysqld-1.example-ndb-mysqld.default

[mysqld]
NodeId=152
Hostname=example-ndb-mysqld-2.example-ndb-mysqld.default

[mysqld]
NodeId=153
Hostname=example-ndb-mysqld-2.example-ndb-mysqld.default

[mysqld]
NodeId=154
Hostname=example-ndb-mysqld-3.example-ndb-mysqld.default

[mysqld]
NodeId=155
Hostname=example-ndb-mysqld-3.example-ndb-mysqld.default

[mysqld]
NodeId=156
Hostname=example-ndb-mysqld-4.example-ndb-mysqld.default

[mysqld]
NodeId=157
Hostname=example-ndb-mysqld-4.example-ndb-mysqld.default

# API sections to be used by generic NDBAPI applications
[api]
NodeId=158

[api]
NodeId=159

[api]
NodeId=160

`
	if configString != expectedConfigString {
		t.Error("The generated config string does not match the expected value")
		t.Errorf("Expected :\n%s\n", expectedConfigString)
		t.Errorf("Generated :\n%s\n", configString)
	}
}

func Test_GetConfigString_withOldConfigSummary(t *testing.T) {

	ndb := testutils.NewTestNdb("default", "example-ndb", 4)
	ndb.Spec.FreeAPISlots = 1
	ndb.Spec.MysqlNode.NodeCount = 1
	ndb.Spec.MysqlNode.MaxNodeCount = 2
	oldConfigSummary := &ConfigSummary{
		MySQLClusterConfigVersion: 3,
		NumOfDataNodes:            2,
	}

	configString, err := GetConfigString(ndb, oldConfigSummary)
	if err != nil {
		t.Errorf("Failed to generate config string from Ndb : %s", err)
	}

	expectedConfigString := `# Auto generated config.ini - DO NOT EDIT

[system]
ConfigGenerationNumber=4
Name=example-ndb



[ndbd default]
NoOfReplicas=2
# Use a fixed ServerPort for all data nodes
ServerPort=1186

[tcp default]
AllowUnresolvedHostnames=1

[ndb_mgmd]
NodeId=1
Hostname=example-ndb-mgmd-0.example-ndb-mgmd.default
DataDir=/var/lib/ndb/data

[ndb_mgmd]
NodeId=2
Hostname=example-ndb-mgmd-1.example-ndb-mgmd.default
DataDir=/var/lib/ndb/data

[ndbd]
NodeId=3
Hostname=example-ndb-ndbmtd-0.example-ndb-ndbmtd.default
DataDir=/var/lib/ndb/data

[ndbd]
NodeId=4
Hostname=example-ndb-ndbmtd-1.example-ndb-ndbmtd.default
DataDir=/var/lib/ndb/data

[ndbd]
NodeId=5
Hostname=example-ndb-ndbmtd-2.example-ndb-ndbmtd.default
DataDir=/var/lib/ndb/data
NodeGroup=65536

[ndbd]
NodeId=6
Hostname=example-ndb-ndbmtd-3.example-ndb-ndbmtd.default
DataDir=/var/lib/ndb/data
NodeGroup=65536

# Dedicated API section to be used by NDB Operator
[api]
NodeId=147
Dedicated=1

# MySQLD sections to be used exclusively by MySQL Servers
[mysqld]
NodeId=148
Hostname=example-ndb-mysqld-0.example-ndb-mysqld.default

[mysqld]
NodeId=149
Hostname=example-ndb-mysqld-1.example-ndb-mysqld.default

# API sections to be used by generic NDBAPI applications
[api]
NodeId=150

`
	if configString != expectedConfigString {
		t.Error("The generated config string does not match the expected value")
		t.Errorf("Expected :\n%s\n", expectedConfigString)
		t.Errorf("Generated :\n%s\n", configString)
	}
}

func Test_GetMySQLConfigString(t *testing.T) {
	nc := testutils.NewTestNdb("default", "example-ndb", 2)
	nc.Spec.MysqlNode.MyCnf = "config1=value1\nconfig2=value2\n"
	configString, err := GetMySQLConfigString(nc, nil)
	if err != nil {
		t.Errorf("Failed to generate MySQL config string from Ndb : %s", err)
	}

	expectedConfigString := `# Auto generated config.ini - DO NOT EDIT
# ConfigVersion=1

[mysqld]
config1=value1
config2=value2

`
	if configString != expectedConfigString {
		t.Error("The generated config string does not match the expected value")
		t.Errorf("Expected :\n%s\n", expectedConfigString)
		t.Errorf("Generated :\n%s\n", configString)
	}
}
