// Copyright (c) 2022, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"

	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	ndbclientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"

	mgmapiutils "github.com/mysql/ndb-operator/e2e-tests/utils/mgmapi"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbutils"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
)

func getIntStrPtrFromString(value string) *intstr.IntOrString {
	v := intstr.FromString(value)
	return &v
}

func getIntStrPtrFromInt(value int) *intstr.IntOrString {
	v := intstr.FromInt(value)
	return &v
}

var _ = ndbtest.NewOrderedTestCase("Erroneous NdbCluster spec update", func(tc *ndbtest.TestContext) {
	var ns string
	var c clientset.Interface
	var testNdb *v1.NdbCluster
	var ctx context.Context
	var ndbClient ndbclientset.Interface

	ginkgo.BeforeAll(func() {
		ginkgo.By("extracting values from TestContext")
		ns = tc.Namespace()
		c = tc.K8sClientset()
		ctx = tc.Ctx()
		ndbClient = tc.NdbClientset()
		// Create a new NdbCluster resource to be used by the test
		testNdb = testutils.NewTestNdb(ns, "ndb-error-update-test", 2)
		testNdb.Spec.MysqlNode.NodeCount = 1
		testNdb.Spec.DataNode.Config = make(map[string]*intstr.IntOrString)
		testNdb.Spec.ManagementNode.Config = make(map[string]*intstr.IntOrString)
		ginkgo.DeferCleanup(func() {
			ginkgo.By("Delete the NdbCluster resource")
			ndbtest.KubectlDeleteNdbObj(c, testNdb)
		})
	})

	ginkgo.When("Erroneous dataNode config is specified in NdbCluster spec", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.DataNode.Config["DaataMemory"] = getIntStrPtrFromString("200M")
			ndbtest.KubectlApplyNdbObjNoWait(testNdb)
		})

		ginkgo.It("should have updated the NdbCluster status on error", func() {
			// wait till the NdbCluster status is updated with syn error
			ndbutils.ValidateNdbClusterStatusUpdatesDuringSync(
				ctx, tc.NdbClientset(), ns, testNdb.Name, true)
			// Get the NdbCluster resource for K8s
			testNdb, err := ndbClient.MysqlV1().NdbClusters(ns).Get(ctx, testNdb.Name, metav1.GetOptions{})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			gomega.Expect(testNdb.HasSyncError()).To(gomega.BeTrue(), "SyncError not found in Ndb Operator status")
		})

		ginkgo.It("should be able to update the NdbCluster spec on error", func() {
			delete(testNdb.Spec.DataNode.Config, "DaataMemory")
			testNdb.Spec.DataNode.Config["DataMemory"] = getIntStrPtrFromString("100M")
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.It("should have the expected Config Version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)
		})
	})

	ginkgo.When("Erroneous management node config is specified in NdbCluster spec", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.ManagementNode.Config["AarbitrationRank"] = getIntStrPtrFromInt(2)
			ndbtest.KubectlApplyNdbObjNoWait(testNdb)
		})

		ginkgo.It("should have updated the NdbCluster status on error", func() {
			// wait till the NdbCluster status is updated with syn error
			ndbutils.ValidateNdbClusterStatusUpdatesDuringSync(
				ctx, tc.NdbClientset(), ns, testNdb.Name, true)
			// Get the NdbCluster resource for K8s
			testNdb, err := ndbClient.MysqlV1().NdbClusters(ns).Get(ctx, testNdb.Name, metav1.GetOptions{})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			gomega.Expect(testNdb.HasSyncError()).To(gomega.BeTrue(), "SyncError not found in Ndb Operator status")
		})

		ginkgo.It("should be able to update the NdbCluster spec on error", func() {
			delete(testNdb.Spec.ManagementNode.Config, "AarbitrationRank")
			testNdb.Spec.ManagementNode.Config["ArbitrationRank"] = getIntStrPtrFromInt(2)
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.It("should have the expected Config Version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 4)
		})
	})

	ginkgo.When("Erroneous Mysql node config is specified in NdbCluster spec", func() {
		ginkgo.BeforeAll(func() {
			// Generate my.cnf value
			myCnfStr := "[mysqld]\n"
			myCnfStr += "maqx-user-connections=1\n"
			testNdb.Spec.MysqlNode.MyCnf = myCnfStr

			ndbtest.KubectlApplyNdbObjNoWait(testNdb)
		})

		ginkgo.It("should have updated the NdbCluster status on error", func() {
			// wait till the NdbCluster status is updated with syn error
			ndbutils.ValidateNdbClusterStatusUpdatesDuringSync(
				ctx, tc.NdbClientset(), ns, testNdb.Name, true)
			// Get the NdbCluster resource for K8s
			testNdb, err := ndbClient.MysqlV1().NdbClusters(ns).Get(ctx, testNdb.Name, metav1.GetOptions{})
			gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			gomega.Expect(testNdb.HasSyncError()).To(gomega.BeTrue(), "SyncError not found in Ndb Operator status")
		})

		ginkgo.It("should be able to update the NdbCluster spec on error", func() {
			// Generate my.cnf value
			myNewCnfStr := "[mysqld]\n"
			myNewCnfStr += "max-user-connections=1\n"
			testNdb.Spec.MysqlNode.MyCnf = myNewCnfStr
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.It("should have the expected Config Version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 6)
		})
	})
})
