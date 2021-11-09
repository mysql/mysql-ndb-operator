// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package yaml

import (
	"github.com/mysql/ndb-operator/e2e-tests/utils/testfiles"
	"path/filepath"
)

// YamlFile reads the content of a single yaml file as string
func YamlFile(path, file string) string {
	from := filepath.Join(path, file+".yaml")
	data := testfiles.ReadTestFile(from)
	return string(data)
}
