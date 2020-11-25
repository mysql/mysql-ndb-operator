// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"fmt"
	"testing"
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
	if rc.NodeGroupCount != 1 {
		t.Fail()
	}
}
