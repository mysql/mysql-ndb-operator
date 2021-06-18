package e2e

import (
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"github.com/onsi/ginkgo"
	"testing"
)

var _ = ginkgo.SynchronizedBeforeSuite(func() []byte {
	// create the Ndb CRD to be used by the suite
	ndbtest.CreateNdbCRD()
	return nil
},
	func([]byte) {},
)

var _ = ginkgo.SynchronizedAfterSuite(func() {
	// delete the Ndb CRD once the suite is done running
	ndbtest.DeleteNdbCRD()
}, func() {}, 5000)

func Test_NdbBasic(t *testing.T) {
	ndbtest.RunGinkgoSuite(t, "mysql", "MySQL Server Tests", true)
}
