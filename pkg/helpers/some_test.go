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
	"time"
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

// sync result describes how to continue after a synchronization step
type syncResult struct {
	// finished true means that step is completed for this round and sync handler shall exit
	finished bool

	// requeue means that sync handler shall exit but report after duration seconds
	requeue time.Duration

	// error is != nil if an error occured during processing, exit sync handler and retry later
	err error
}

var finishedResult = syncResult{finished: true, requeue: 0, err: nil}

func resultReturn() syncResult {
	return finishedResult
}

func Test_results(t *testing.T) {

	res := resultReturn()
	if resultReturn() != finishedResult {
		t.Fail()
	}

	if !res.finished {
		t.Fail()
	}

}

func Test_array(t *testing.T) {

	ar := make([]int, 0, 15)
	ar[7] = 12
}
