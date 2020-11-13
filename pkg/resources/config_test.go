// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"testing"
)

func Test_GetConfigHashAndGenerationFromConfig(t *testing.T) {

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

	hash, generation := GetConfigHashAndGenerationFromConfig(testini)

	if hash != "asdasdlkajhhnxh=?" {
		t.Fail()
	}
	if generation != 4711 {
		t.Errorf("Wrong generation :" + string(generation))
	}
}
