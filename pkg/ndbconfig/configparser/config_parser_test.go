// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package configparser

import (
	"fmt"
	"os"
	"testing"
)

// ValidateConfigIniSectionCount validates the count of a
// given section in the configIni
func validateConfigIniSectionCount(
	t *testing.T, config ConfigIni, sectionName string, expected int) (validationSuccess bool) {
	t.Helper()
	if actual := config.GetNumberOfSections(sectionName); actual != expected {
		t.Errorf("Expected number of '%s' sections : %d. Actual : %d", sectionName, expected, actual)
		return false
	}

	return true
}

func TestReadInifile(t *testing.T) {

	f, err := os.Create("test.txt")
	if err != nil {
		fmt.Println(err)
		return
	}

	// Close and remove the file before exit
	defer func() {
		f.Close()
		os.Remove("test.txt")
	}()

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
	
	[mysqld]
	NodeId=1
	Hostname=example-ndb-0.example-ndb.svc.default-namespace.com
	
	[mysqld]`

	l, err := f.WriteString(testini)
	if err != nil {
		t.Error(err)
		return
	}

	if l == 0 {
		t.Fail()
		return
	}

	c, err := ParseFile("test.txt")

	if err != nil {
		t.Error(err)
		return
	}

	if c == nil || len(c) == 0 {
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
	expectedNdbdServerPort := "1186"
	ndbdServerPort := c.GetValueFromSection("ndbd", "ServerPort")
	if expectedNdbdServerPort != ndbdServerPort {
		t.Errorf("Expected ndbd's ServerPort : %s but got %s", expectedNdbdServerPort, ndbdServerPort)
	}

	expectedMgmdHostname := "example-ndb-0.example-ndb.svc.default-namespace.com"
	mgmdHostname := c.GetValueFromSection("ndb_mgmd", "Hostname")
	if expectedMgmdHostname != mgmdHostname {
		t.Errorf("Expected mgmd's Hostname : %s but got %s", expectedMgmdHostname, mgmdHostname)
	}

	if !validateConfigIniSectionCount(t, c, "api", 0) {
		t.Error("The section 'api' was parsed despite being commented out")
	}

	validateConfigIniSectionCount(t, c, "ndbd", 1)
	validateConfigIniSectionCount(t, c, "ndb_mgmd", 1)
	validateConfigIniSectionCount(t, c, "mysqld", 2)
	validateConfigIniSectionCount(t, c, "ndbd default", 1)
	validateConfigIniSectionCount(t, c, "tcp default", 1)
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
