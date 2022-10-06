// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package configparser

import (
	"strings"
	"testing"
)

// ValidateConfigIniSectionCount validates the count of a
// given section in the configIni
func validateConfigIniSectionCount(
	t *testing.T, config ConfigIni, sectionName string, expected int) {
	t.Helper()
	if actual := config.GetNumberOfSections(sectionName); actual != expected {
		t.Errorf("Expected number of '%s' sections : %d. Actual : %d", sectionName, expected, actual)
	}
}

func validateConfigKeys(t *testing.T, config ConfigIni, sectionName string, expectedKeys [][]string) {
	t.Helper()

	sectionGrp := config.GetAllSections(sectionName)
	if sectionGrp == nil {
		t.Errorf("Failed to find section %q", sectionName)
		return
	}

	if len(sectionGrp) != len(expectedKeys) {
		t.Errorf("Found Unexpected number of Section %q", sectionName)
		return
	}

	for i, keys := range expectedKeys {
		section := sectionGrp[i]
		if len(section) != len(keys) {
			t.Errorf("Found unexpected number of keys in section %q group %d : %v", sectionName, i, section)
		}
		for _, key := range keys {
			if _, exists := section.GetValue(key); !exists {
				t.Errorf("Failed to find expected key %q in section %q : %v", key, sectionName, section)
			}
		}
	}
}

func validateErrors(t *testing.T, configStr string, expectedError string) {
	t.Helper()

	if _, err := ParseString(configStr); err != nil && !strings.Contains(err.Error(), expectedError) {
		t.Errorf("ParseString throw an unexpected error : %s", err)
	} else if err == nil {
		t.Errorf("ParseString was expected to throw an error but it did not")
	}
}

func Test_ParseString(t *testing.T) {

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
    # [api]

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

	[ndbd]
	NodeId=2
	Hostname=example-ndb-1.example-ndb.svc.default-namespace.com
	DataDir=/var/lib/ndb
	
	[mysqld]
	NodeId=1
	Hostname=example-ndb-0.example-ndb.svc.default-namespace.com
	
	[mysqld]`

	c, err := ParseString(testini)

	if err != nil {
		t.Error(err)
		return
	}

	if c == nil {
		t.Fatal("configIni is empty")
		return
	}

	// Verify number of groups
	expectedNumOfGroups := 6
	numOfGroups := len(c)
	if numOfGroups != expectedNumOfGroups {
		t.Errorf("Expected %d groups but got %d groups", expectedNumOfGroups, numOfGroups)
	}

	t.Log("Iterating")
	for s, grp := range c {
		for _, sec := range grp {
			t.Log("[" + s + "]")
			for key, val := range sec {
				t.Log(key + ": " + val)
			}
		}
	}

	// Verify that values are parsed as expected
	// TODO: Verify more keys
	expectedDataMemory := "80M"
	ndbdDataMemory := c.GetValueFromSection("ndbd default", "DataMemory")
	if expectedDataMemory != ndbdDataMemory {
		t.Errorf("Expected ndbd's DataMemory : %s but got %s", expectedDataMemory, ndbdDataMemory)
	}

	expectedMgmdHostname := "example-ndb-0.example-ndb.svc.default-namespace.com"
	mgmdHostname := c.GetValueFromSection("ndb_mgmd", "Hostname")
	if expectedMgmdHostname != mgmdHostname {
		t.Errorf("Expected mgmd's Hostname : %s but got %s", expectedMgmdHostname, mgmdHostname)
	}

	validateConfigIniSectionCount(t, c, "ndbd", 2)
	validateConfigIniSectionCount(t, c, "ndb_mgmd", 1)
	validateConfigIniSectionCount(t, c, "mysqld", 2)
	validateConfigIniSectionCount(t, c, "ndbd default", 1)
	validateConfigIniSectionCount(t, c, "tcp default", 1)

	// The section 'api' should not be parsed as it is commented out
	validateConfigIniSectionCount(t, c, "api", 0)

	validateConfigKeys(t, c, "header", [][]string{
		{"ConfigHash"},
	})
	validateConfigKeys(t, c, "ndbd default", [][]string{
		{"NoOfReplicas", "DataMemory", "ServerPort", "StartPartialTimeout", "StartPartitionedTimeout"},
	})
	validateConfigKeys(t, c, "tcp default", [][]string{
		{"AllowUnresolvedHostnames"},
	})
	validateConfigKeys(t, c, "ndb_mgmd", [][]string{
		{"NodeId", "Hostname", "DataDir"},
	})
	validateConfigKeys(t, c, "ndbd", [][]string{
		{"NodeId", "Hostname", "DataDir", "ServerPort"},
		{"NodeId", "Hostname", "DataDir"},
	})
	validateConfigKeys(t, c, "mysqld", [][]string{
		{"NodeId", "Hostname"}, {},
	})

	// Validate Errors
	validateErrors(t, "#\n#\n\n\n[test]\nkey", "Format error at line 6")
	validateErrors(t, "[test]\nkey=", "Format error at line 2")
	validateErrors(t, "#\n#\n\n[test", "Incomplete section name at line 4")
	validateErrors(t, "#\n#\n\n\n[test]key", "Incomplete section name at line 5")
	validateErrors(t, "#\n\nkey", "Non-empty line without section at line 3")
	validateErrors(t, "#\n\nkey=value", "Non-empty line without section at line 3")
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

	validateConfigIniSectionCount(t, config, "ndbd", 4)
	validateConfigIniSectionCount(t, config, "mgmd", 2)
	validateConfigIniSectionCount(t, config, "mysqld", 0)
}

func configEqualTester(t *testing.T, config1, config2 string, expected bool, desc string) {
	t.Helper()
	if ConfigEqual(config1, config2) != expected {
		t.Errorf("Testcase %q failed", desc)
	}
}

func Test_ConfigEqual(t *testing.T) {
	equal := true
	configEqualTester(t,
		"[ndbd default]\nDataMemory=20M\nNoOfReplica=2\n",
		"[ndbd default]\nDataMemory=20M\nNoOfReplica=2\n", equal,
		"equal config")
	configEqualTester(t,
		"[ndbd default]\nNoOfReplica=2\nDataMemory=20M\n",
		"[ndbd default]\nDataMemory=20M\nNoOfReplica=2\n", equal,
		"config params out of order but configs equal")
	configEqualTester(t,
		"[ndbd default]\nDataMemory=20M\nNoOfReplica=2\n",
		"[ndbd]\nDataMemory=20M\nNoOfReplica=2\n", !equal,
		"different sections with similar config")
	configEqualTester(t,
		"[ndbd]\nDataMemory=20M\nNoOfReplica=2\n[mysqld]\nndbcluster=true\nuser=root\n",
		"[ndbd]\nDataMemory=20M\nNoOfReplica=2\n[mysqld]\nndbcluster=true\nuser=root\n", equal,
		"equal config with multiple sections")
	configEqualTester(t,
		"[ndbd]\nDataMemory=20M\nNoOfReplica=2\n[mysqld]\nndbcluster=true\nuser=root\n",
		"[mysqld]\nndbcluster=true\nuser=root\n[ndbd]\nDataMemory=20M\nNoOfReplica=2\n", equal,
		"equal config with multiple sections that are out of order")
	configEqualTester(t,
		"[ndbd]\nNoOfReplica=2\nDataMemory=20M\n[mysqld]\nuser=root\nndbcluster=true\n",
		"[mysqld]\nndbcluster=true\nuser=root\n[ndbd]\nDataMemory=20M\nNoOfReplica=2\n", equal,
		"equal config with multiple sections and params that are out of order")
	configEqualTester(t,
		"[ndbd]\nNoOfReplica=2\nDataMemory=20M\n[mysqld]\nuser=mysql\nndbcluster=true\n",
		"[mysqld]\nndbcluster=true\nuser=root\n[ndbd]\nDataMemory=20M\nNoOfReplica=2\n", !equal,
		"unequal config with multiple sections and params that are out of order")
	configEqualTester(t,
		"[mysqld]\nuser=mysql\nndbcluster=true\n[mysqld]\nuser=root\nndbcluster=true\n",
		"[mysqld]\nndbcluster=true\nuser=root\n[mysqld]\nuser=mysql\nndbcluster=true\n", equal,
		"equal configs with multiple sections with same name")
	configEqualTester(t,
		"[mysqld]\nuser=mysql\nndbcluster=true\n",
		"[mysqld]\nndbcluster=true\nuser=root\n[mysqld]\nuser=mysql\nndbcluster=true\n", !equal,
		"unequal configs with one config having more sections")
	configEqualTester(t,
		"[mysqld]\nuser=mysql\nndbcluster=true\n[mysqld]\nuser=mysql\nndbcluster=true\n",
		"[mysqld]\nndbcluster=true\nuser=mysql\n[mysqld]\nuser=mysql\nndbcluster=true\n", equal,
		"equal configs with having multiple, equal sections")
	configEqualTester(t,
		"[mysqld]\nuser=mysql\nndbcluster=true\n[mysqld]\nuser=mysql\nndbcluster=true\n",
		"[mysqld]\nndbcluster=true\nuser=mysql\n[mysqld]\nuser=root\nndbcluster=true\n", !equal,
		"unequal configs with having multiple sections")
}
