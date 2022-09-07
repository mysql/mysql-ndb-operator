// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/mysql/ndb-operator/pkg/mgmapi"

	"github.com/mysql/ndb-operator/e2e-tests/utils/mgmapi"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

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

var _ = ndbtest.NewOrderedTestCase("MySQL Cluster config update", func(tc *ndbtest.TestContext) {
	var ns string
	var c clientset.Interface
	var testNdb *v1alpha1.NdbCluster

	ginkgo.BeforeAll(func() {
		ginkgo.By("extracting values from TestContext")
		ns = tc.Namespace()
		c = tc.K8sClientset()
		// Create a new NdbCluster resource to be used by the test
		testNdb = testutils.NewTestNdb(ns, "ndb-config-test", 2)
		testNdb.Spec.Mysqld.NodeCount = 1
		testNdb.Spec.DataNode.Config = make(map[string]*intstr.IntOrString)
		testNdb.Spec.ManagementNodeConfig = make(map[string]*intstr.IntOrString)
		ginkgo.DeferCleanup(func() {
			ginkgo.By("Delete the NdbCluster resource")
			ndbtest.KubectlDeleteNdbObj(c, testNdb)
		})
	})

	ginkgo.When("DataNode Config is specified in NdbCluster spec", func() {
		ginkgo.BeforeAll(func() {
			// Set a 200M memory to the mgmclient resource and create the object
			testNdb.Spec.DataNode.Config["DataMemory"] = getIntStrPtrFromString("200M")
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.It("should start the datanodes' with the specified config", func() {
			expectedDataMemory := uint64(200 * 1024 * 1024)
			mgmapiutils.ForEachConnectedNodes(c, testNdb, mgmapi.NodeTypeNDB, func(mgmClient mgmapi.MgmClient, nodeId int) {
				gomega.Expect(mgmClient.GetDataMemory(nodeId)).To(gomega.Equal(expectedDataMemory))
			})
		})

		ginkgo.It("should have started the management nodes with default config", func() {
			mgmapiutils.ForEachConnectedNodes(c, testNdb, mgmapi.NodeTypeMGM, func(mgmClient mgmapi.MgmClient, nodeId int) {
				gomega.Expect(mgmClient.GetMgmdArbitrationRank()).To(gomega.BeEquivalentTo(1))
			})
		})

		ginkgo.It("should have the expected Config Version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 1)
		})
	})

	ginkgo.When("MySQL Cluster's config is updated", func() {
		ginkgo.BeforeAll(func() {
			// Update the DataMemory to 300M
			testNdb.Spec.DataNode.Config["DataMemory"] = getIntStrPtrFromString("300M")
			// Update ArbitrationRank
			testNdb.Spec.ManagementNodeConfig["ArbitrationRank"] = getIntStrPtrFromInt(2)
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.It("should have updated the update the DataNodes' config", func() {
			expectedDataMemory := uint64(300 * 1024 * 1024)
			mgmapiutils.ForEachConnectedNodes(c, testNdb, mgmapi.NodeTypeNDB, func(mgmClient mgmapi.MgmClient, nodeId int) {
				gomega.Expect(mgmClient.GetDataMemory(nodeId)).To(gomega.Equal(expectedDataMemory))
			})
		})

		ginkgo.It("should have updated the update the management nodes' config", func() {
			mgmapiutils.ForEachConnectedNodes(c, testNdb, mgmapi.NodeTypeMGM, func(mgmClient mgmapi.MgmClient, nodeId int) {
				gomega.Expect(mgmClient.GetMgmdArbitrationRank()).To(gomega.BeEquivalentTo(2))
			})
		})

		ginkgo.It("should have updated the MySQL Cluster Config Version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)
		})
	})
})
