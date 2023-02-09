// Copyright (c) 2022, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"

	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	ndbclientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = ndbtest.NewOrderedTestCase("NdbCluster validation", func(tc *ndbtest.TestContext) {
	var ns string
	var ctx context.Context
	var testNdb *v1.NdbCluster
	var ndbclient ndbclientset.Interface

	ginkgo.BeforeAll(func() {
		ginkgo.By("extracting values from TestContext")
		ns = tc.Namespace()
		ctx = tc.Ctx()
		ndbclient = tc.NdbClientset()
	})

	ginkgo.BeforeEach(func() {
		// Create a new NdbCluster resource to be used by the test
		testNdb = testutils.NewTestNdb(tc.Namespace(), "validation-test", 2)
	})

	ginkgo.Context("creating a NdbCluster with an invalid number of data nodes should fail", func() {

		ginkgo.Specify("a data node count that is not a multiple of redundancyLevel", func() {
			testNdb.Spec.DataNode.NodeCount = 1
			_, err := ndbclient.MysqlV1().NdbClusters(ns).Create(ctx, testNdb, metav1.CreateOptions{})
			ndbtest.ExpectError(err)
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(
				"spec.dataNode.nodeCount: Invalid value: 1: spec.dataNode.nodeCount should be a multiple of the spec.redundancyLevel(=2)"))
		})

		ginkgo.Specify("a data node count that exceeds the maximum", func() {
			testNdb.Spec.DataNode.NodeCount = 145
			_, err := ndbclient.MysqlV1().NdbClusters(ns).Create(ctx, testNdb, metav1.CreateOptions{})
			ndbtest.ExpectError(err)
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(
				"spec.dataNode.nodeCount: Invalid value: 145: spec.dataNode.nodeCount in body should be less than or equal to 144"))
		})
	})

	ginkgo.When("a disallowed data node config param is specified in NdbCluster spec", func() {
		ginkgo.It("should throw appropriate errors", func() {
			testNdb.Spec.DataNode.Config = map[string]*intstr.IntOrString{
				"NoOfReplicas": getIntStrPtrFromString("3"),
				"HostName":     getIntStrPtrFromString("localhost"),
				"dataDir":      getIntStrPtrFromString("/tmp"),
			}
			_, err := ndbclient.MysqlV1().NdbClusters(ns).Create(ctx, testNdb, metav1.CreateOptions{})
			ndbtest.ExpectError(err)
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(
				"spec.dataNode.config.NoOfReplicas: Forbidden: config param \"NoOfReplicas\" is not allowed in spec.dataNode.config"))
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(
				"spec.dataNode.config.dataDir: Forbidden: config param \"dataDir\" is not allowed in spec.dataNode.config"))
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(
				"spec.dataNode.config.HostName: Forbidden: config param \"HostName\" is not allowed in spec.dataNode.config"))
		})
	})

	ginkgo.When("a disallowed management node config param is specified in NdbCluster spec", func() {
		ginkgo.It("should throw appropriate errors", func() {
			testNdb.Spec.ManagementNode.Config = map[string]*intstr.IntOrString{
				"PortNumber": getIntStrPtrFromString("33333"),
				"HostName":   getIntStrPtrFromString("localhost"),
			}
			_, err := ndbclient.MysqlV1().NdbClusters(ns).Create(ctx, testNdb, metav1.CreateOptions{})
			ndbtest.ExpectError(err)
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(
				"spec.managementNode.config.PortNumber: Forbidden: config param \"PortNumber\" is not allowed in spec.managementNode.config"))
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(
				"spec.managementNode.config.HostName: Forbidden: config param \"HostName\" is not allowed in spec.managementNode.config"))
		})
	})
})
