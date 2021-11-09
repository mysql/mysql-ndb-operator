// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// testfiles package has the constants and methods required to read a file from the project directory

package testfiles

import (
	"fmt"
	"github.com/onsi/ginkgo"
	"io/ioutil"
	"path/filepath"
)

const (
	root = "../../.."
)

// ReadTestFile looks for the file relative to the configured root directory.
func ReadTestFile(filePath string) []byte {
	fullPath := filepath.Join(root, filePath)
	data, err := ioutil.ReadFile(fullPath)
	if err != nil {
		ginkgo.Fail(fmt.Sprintf("Failed to read file %q : %v", filePath, err), 1)
	}
	return data
}
