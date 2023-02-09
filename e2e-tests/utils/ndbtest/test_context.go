// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbtest

import (
	"context"
	"fmt"

	"github.com/mysql/ndb-operator/e2e-tests/utils/k8sutils"
	ndbclient "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
	ginkgo "github.com/onsi/ginkgo/v2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	klog "k8s.io/klog/v2"
)

// TestContext has the necessary information to run a test.
// When a test case is described using the NewTestCase method,
// a new TestContext will be initialized automatically through
// Before/After-Each blocks and is passed to the test spec.
// TestContext will be initialized and accessible only inside
// the child nodes (i.e. It, BeforeEach, AfterEach, etc.) of
// the test case. Attempting to retrieve the field values
// outside the child nodes will return nil.
type TestContext struct {
	ctx          context.Context
	namespace    string
	k8sClientset kubernetes.Interface
	ndbClientset ndbclient.Interface
	initDone     bool
	// bool flag to track if Ndb Operator was successfully installed
	ndbOperatorInstalled bool
}

// NewTestContext returns a new TestContext
// with the clientsets and ctx initialised.
func NewTestContext() *TestContext {
	if !ndbTestSuite.setupDone {
		panic("Ndb TestSuite is not initialised")
	}
	return &TestContext{
		ctx:          ndbTestSuite.ctx,
		k8sClientset: ndbTestSuite.clientset,
		ndbClientset: ndbTestSuite.ndbClientset,
	}
}

func (tc *TestContext) Ctx() context.Context {
	return tc.ctx
}

func (tc *TestContext) Namespace() string {
	return tc.namespace
}

func (tc *TestContext) K8sClientset() kubernetes.Interface {
	return tc.k8sClientset
}

func (tc *TestContext) NdbClientset() ndbclient.Interface {
	return tc.ndbClientset
}

// init generates a new namespace to be used by the test spec,
// creates it in the K8s cluster.
func (tc *TestContext) init(suiteName string) {

	if tc.initDone {
		panic("TestContext is already initialised")
	}

	// Generate namespace name and create it in K8s
	var namespace string
	if ndbTestSuite.createNamespaces {
		for {
			// Generate a name for the namespace of form '<suite-name>-<uniqueId>'
			namespace = fmt.Sprintf("%s-%d", suiteName, <-ndbTestSuite.uniqueId)
			klog.Infof("Generated name %q for TestContext namespace", namespace)
			// Create it
			_, err := k8sutils.CreateNamespace(tc.ctx, tc.k8sClientset, namespace)
			if err == nil {
				// create succeeded
				klog.Infof("Successfully created the namespace %q in K8s", namespace)
				break
			}
			if !errors.IsAlreadyExists(err) {
				// It failed due to some error other than StatusReasonAlreadyExists.
				// No need to retry
				ginkgo.Fail(fmt.Sprintf("Failed to create namespace %q : %s", namespace, err))
			}
			// Namespace already exists.
			// Retry with another unique id.
			klog.Infof("Namespace %q already exists. Retrying with another name", namespace)
		}
		// set namespace to TestContext
		tc.namespace = namespace
	}

	tc.initDone = true

	// Reset ndbOperatorInstalled flag
	tc.ndbOperatorInstalled = false
}

// cleanup deletes the namespace created by the init method.
func (tc *TestContext) cleanup() {
	if ndbTestSuite.createNamespaces {
		ginkgo.By("Deleting the namespace")
		err := k8sutils.DeleteNamespace(tc.ctx, tc.k8sClientset, tc.namespace)
		ExpectNoError(err,
			fmt.Sprintf("Failed to delete namespace %q during TestContext cleanup", tc.namespace))
	}
	tc.initDone = false
}
