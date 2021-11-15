// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"testing"
)

func Test_MySQLSuite(t *testing.T) {
	ndbtest.RunGinkgoSuite(t, "mysql", "MySQL Server Tests",
		true, true, []string{ndbtest.NdbClusterCRD})
}
