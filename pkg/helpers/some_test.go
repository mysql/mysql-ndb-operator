// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package helpers

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func Test_regex(t *testing.T) {

	content := []byte(`
	# comment line
	option1: value1
	#option2: value2
	other weird test text
	[group name]
	# another comment line
	option3: value3
`)

	// Regex pattern captures "key: value" pair from the content.
	pattern := regexp.MustCompile(`(?m)(?P<key>\w+):\s+(?P<value>\w+)$`)

	// Template to convert "key: value" to "key=value" by
	// referencing the values captured by the regex pattern.
	template := []byte("$key=$value\n")

	result := []byte{}

	// For each match of the regex in the content.
	for _, submatches := range pattern.FindAllSubmatchIndex(content, -1) {
		// Apply the captured submatches to the template and append the output
		// to the result.
		result = pattern.Expand(result, template, content, submatches)
	}

	lines := strings.Split(string(result), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 1 {
			continue
		}
		split := strings.Split(string(line), "=")
		if len(split) != 2 {
			t.Error(errors.New("Format error >" + string(line) + "<"))
		}

		fmt.Println(split[0] + ": " + split[1])
	}

	t.Fail()
}
