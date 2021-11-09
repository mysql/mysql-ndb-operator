// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbtest

import (
	"fmt"
	"github.com/onsi/ginkgo"
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

		// Run the body
		body(tc)
	})
}
