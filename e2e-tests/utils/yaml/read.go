// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package yaml

import (
	"os"
	"path/filepath"

	"k8s.io/klog"

	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/testfiles"
)

// YamlFile reads the content of a single yamle file as string
// Read happens from a test files path which needs to first
// be registered with testfiles.AddFileSource()
func YamlFile(path, file string) string {
	from := filepath.Join(path, file+".yaml")
	data, err := testfiles.Read(from)
	if err != nil {
		dir, _ := os.Getwd()
		klog.Infof("Maybe in wrong directory %s", dir)
		framework.Fail(err.Error())
	}
	return string(data)
}
