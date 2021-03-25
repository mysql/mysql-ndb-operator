package util

import (
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"testing"
)

func Test_Util(t *testing.T) {
	ndbtest.RunGinkgoSuite(t, "utils-suite", "Utils test suite", false)
}
