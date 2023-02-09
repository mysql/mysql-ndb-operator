// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbtest

import (
	"fmt"
	"path/filepath"
	"sync"

	podutils "github.com/mysql/ndb-operator/e2e-tests/utils/pods"
	"github.com/mysql/ndb-operator/e2e-tests/utils/testfiles"
	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/labels"
	klog "k8s.io/klog/v2"
)

// newTestCaseImpl is a wrapper around the ginkgo.Describe block with
// additional BeforeEach/BeforeAll blocks that initialize the
// TestContext to be used by the test spec.
func newTestCaseImpl(name string, body func(tc *TestContext), ordered bool) bool {
	desc := fmt.Sprintf("[TestCase : %q]", name)

	// Use BeforeEach for non-ordered and BeforeAll for ordered testcase
	BeforeFunc := ginkgo.BeforeEach
	if ordered {
		BeforeFunc = ginkgo.BeforeAll
	}

	// Define a func that does the required setup using
	// the BeforeFunc and then executes the body func.
	testCaseFunc := func() {
		// TestContext to pass to the testcases. It will be
		// shared by the specs that are defined inside the body.
		// A new Namespace will be created for each spec as the
		// specs are usually run one by one and the init/cleanup
		// methods delete the previous namespace and create a
		// new one in between the specs.
		tc := NewTestContext()

		// Setup BeforeEach to init/cleanup TestContexts
		BeforeFunc(func() {
			ginkgo.By("Initialising TestContext")
			tc.init(ndbTestSuite.suiteName)

			ginkgo.DeferCleanup(func() {
				ginkgo.By("Cleaning up TestContext")
				tc.cleanup()
			})
		})

		// Set up NDB Operator for the testcase. This BeforeEach also
		// starts a couple of goroutines to monitor the Operator for
		// any crashes and to stream/save the operator pod logs.
		BeforeFunc(func() {
			// Ginkgo should wait for the goroutines started by this
			// BeforeEach before moving on to the next testcase.
			// Use a sync.WaitGroup to track and wait for those
			// goroutines via ginkgo.DeferCleanup.
			var wg sync.WaitGroup
			ginkgo.DeferCleanup(func() {
				// Wait for the go routines to complete
				wg.Wait()
			})

			// Install operator and on success Set up the uninstall method as Cleanup
			installNdbOperator(tc)
			// Operator installed successfully.
			// Set up the uninstall method as the Cleanup method.
			ginkgo.DeferCleanup(uninstallNdbOperator, tc)

			// Extract name of the operator pod
			ctx := tc.ctx
			namespace := tc.namespace
			clientset := tc.k8sClientset
			podName := podutils.GetPodNameWithLabel(
				ctx, clientset, namespace, labels.Set{"app": "ndb-operator"}.AsSelector())
			klog.Infof("Ndb Operator is running in pod %q", podName)

			// Start a go routine to watch NDB Operator for crashes
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer ginkgo.GinkgoRecover()

				// Watch for pod errors
				podFailed := podutils.WatchForPodError(ctx, clientset, namespace, podName)
				gomega.Expect(podFailed).To(gomega.BeFalse(), "Ndb Operator pod failed")
			}()

			// Start a go routine that streams and stores the operator pod logs
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer ginkgo.GinkgoRecover()

				// Collect the pod logs and add them as a spec report entry
				operatorLogs := podutils.CollectPodLogs(ctx, clientset, namespace, podName)
				ginkgo.AddReportEntry("NDB Operator pod log", operatorLogs, ginkgo.ReportEntryVisibilityFailureOrVerbose)
			}()
		})

		// Run the body
		body(tc)
	}

	// Execute the testcase within a Describe block
	if ordered {
		return ginkgo.Describe(desc, ginkgo.Ordered, testCaseFunc)
	} else {
		return ginkgo.Describe(desc, testCaseFunc)
	}
}

// NewTestCase runs the given testcase inside an unordered Describe container
func NewTestCase(name string, body func(tc *TestContext)) bool {
	return newTestCaseImpl(name, body, false)
}

// NewOrderedTestCase runs the given testcase inside an ordered Describe container
func NewOrderedTestCase(name string, body func(tc *TestContext)) bool {
	return newTestCaseImpl(name, body, true)
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
	}, func([]byte) {})

	// Setup AfterSuite for the suite
	ginkgo.SynchronizedAfterSuite(func() {
		// Delete all the CRDs passed via crdList before starting the suite
		for _, crdPath := range crdList {
			ginkgo.By(fmt.Sprintf("Deleting CRD from %s", filepath.Base(crdPath)))
			RunKubectl(DeleteCmd, "", string(testfiles.ReadTestFile(crdPath)))
		}
	}, func() {})
}
