// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbtest

import (
	"fmt"
	"github.com/mysql/ndb-operator/e2e-tests/utils/testfiles"
	"github.com/onsi/ginkgo"
	"path/filepath"
)

// NewTestCase is a wrapper around the ginkgo.Describe block with
// additional BeforeEach and AfterEach blocks that initialize the
// TestContext to be used by the test spec.
func NewTestCase(name string, body func(tc *TestContext)) bool {
	desc := fmt.Sprintf("[TestCase : %q]", name)
	return ginkgo.Describe(desc, func() {
		// TestContext to pass to the testcases. It will be
		// shared by the specs that are defined inside the body.
		// A new Namespace will be created for each spec as the
		// specs are usually run one by one and the init/cleanup
		// methods delete the previous namespace and create a
		// new one in between the specs.
		tc := newTestContext()

		// Setup before and after each to init/cleanup TestContexts
		ginkgo.BeforeEach(func() {
			ginkgo.By("Initialising TestContext")
			tc.init(ndbTestSuite.suiteName)
		})
		ginkgo.AfterEach(func() {
			ginkgo.By("Cleaning up TestContext")
			tc.cleanup()
		})

		// Setup Before/AfterEach methods to install/uninstall NDB Operator
		ginkgo.BeforeEach(func() {
			installNdbOperator(tc)
		})
		ginkgo.AfterEach(func() {
			uninstallNdbOperator(tc)
		})

		// Run the body
		body(tc)
	})
}

// setupBeforeAfterSuite registers Before and After Suite
// methods for the current Suite. Any CRD requested by the
// suite is created and cleaned up by these methods.
func setupBeforeAfterSuite(crdList []string) {
	// Setup BeforeSuite for the suite
	ginkgo.SynchronizedBeforeSuite(func() []byte {
		// Install all the CRDs passed via crdList before starting the suite
		for _, crdPath := range crdList {
			ginkgo.By(fmt.Sprintf("Creating CRD from %s", filepath.Base(crdPath)))
			RunKubectl(CreateCmd, "", string(testfiles.ReadTestFile(crdPath)))
		}
		return nil
	}, func([]byte) {}, 600)

	// Setup AfterSuite for the suite
	ginkgo.SynchronizedAfterSuite(func() {
		// Install all the CRDs passed via crdList before starting the suite
		for _, crdPath := range crdList {
			ginkgo.By(fmt.Sprintf("Deleting CRD from %s", filepath.Base(crdPath)))
			RunKubectl(DeleteCmd, "", string(testfiles.ReadTestFile(crdPath)))
		}
	}, func() {}, 600)
}
