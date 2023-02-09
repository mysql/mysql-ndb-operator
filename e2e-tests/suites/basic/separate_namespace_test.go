// Copyright (c) 2022, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"

	"github.com/mysql/ndb-operator/e2e-tests/utils/k8sutils"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	sfset_utils "github.com/mysql/ndb-operator/e2e-tests/utils/statefulset"

	ginkgo "github.com/onsi/ginkgo/v2"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = ndbtest.NewOrderedTestCase("NdbCluster in a separate namespace", func(tc *ndbtest.TestContext) {
	var ctx context.Context
	var ns, ndbNamespace string
	var c clientset.Interface
	var testNdb *v1.NdbCluster

	ginkgo.BeforeEach(func() {
		ginkgo.By("extracting values from TestContext")
		ns = tc.Namespace()
		c = tc.K8sClientset()
		ctx = tc.Ctx()

		ndbNamespace = ns + "-deux"
		testNdb = testutils.NewTestNdb(ndbNamespace, "separate-namespace-test", 2)
	})

	ginkgo.When("the NdbCluster resource is created in a namespace different than that of the operator", func() {

		ginkgo.BeforeEach(func() {
			// Create the namespace for the test
			_, err := k8sutils.CreateNamespace(ctx, c, ndbNamespace)
			ndbtest.ExpectNoError(err, "create namespace %q failed", ndbNamespace)
			ginkgo.DeferCleanup(func() {
				err := k8sutils.DeleteNamespace(ctx, c, ndbNamespace)
				ndbtest.ExpectNoError(err, "delete namespace %q failed", ndbNamespace)
			})
		})

		ginkgo.AfterEach(func() {
			ndbtest.KubectlDeleteNdbObj(c, testNdb)
		})

		ginkgo.It("should deploy MySQL cluster nodes in the specified namespace", func() {
			ndbtest.KubectlApplyNdbObj(c, testNdb)
			sfset_utils.ExpectHasReplicas(c, ndbNamespace, testNdb.GetWorkloadName(constants.NdbNodeTypeMgmd), 2)
			sfset_utils.ExpectHasReplicas(c, ndbNamespace, testNdb.GetWorkloadName(constants.NdbNodeTypeNdbmtd), 2)
			sfset_utils.ExpectHasReplicas(c, ndbNamespace, testNdb.GetWorkloadName(constants.NdbNodeTypeMySQLD), 2)
		})
	})
})
