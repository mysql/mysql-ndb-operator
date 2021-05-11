package e2e

import (
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"testing"
)

func Test_NdbBasic(t *testing.T) {
	ndbtest.RunGinkgoSuite(t, "mysql", "MySQL Server Tests", true)
}
