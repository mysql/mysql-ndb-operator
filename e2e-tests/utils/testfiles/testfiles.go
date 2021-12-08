// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// testfiles package has the constants and methods required to read a file from the project directory

package testfiles

import (
	"fmt"
	"github.com/onsi/gomega"
	"io/ioutil"
	"k8s.io/klog"
	"path/filepath"
	"runtime"
)

// project root directory
var root string

func init() {
	var _, filename, _, _ = runtime.Caller(0)
	// This file is at project-root/e2e-tests/utils/testfiles
	root = filepath.Join(filepath.Dir(filename), "../../..")
	klog.Infof("Project root directory at %q", root)
}

// GetAbsPath returns the absolute path of the given
// path that was relative to the project root directory.
func GetAbsPath(path string) string {
	return filepath.Join(root, path)
}

// ReadTestFile looks for the file relative to the configured root directory.
func ReadTestFile(filePath string) []byte {
	fullPath := filepath.Join(root, filePath)
	data, err := ioutil.ReadFile(fullPath)
	gomega.ExpectWithOffset(1, err).Should(
		gomega.Succeed(), fmt.Sprintf("Failed to read file %q", filePath))
	return data
}