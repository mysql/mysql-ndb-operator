// Copyright (c) 2022, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"strings"

	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	mgmapiutils "github.com/mysql/ndb-operator/e2e-tests/utils/mgmapi"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	"k8s.io/client-go/kubernetes"
)

// expectMySQLClusterVersion verifies if all the nodes in the MySQL Cluster
// run the version specified in the NdbCluster resource's image value.
func expectMySQLClusterVersion(clientset kubernetes.Interface, nc *v1.NdbCluster) {
	mgmClient := mgmapiutils.ConnectToMgmd(clientset, nc)
	status, err := mgmClient.GetStatus()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	expectedVersion := strings.Split(nc.Spec.Image, ":")[1]
	for _, node := range status {
		if node.IsConnected {
			gomega.Expect(node.SoftwareVersion).To(gomega.Equal(expectedVersion))
		}
	}
}

var _ = ndbtest.NewOrderedTestCase("MySQL Cluster minor upgrade", func(tc *ndbtest.TestContext) {
	var ns string
	var c kubernetes.Interface
	var testNdb *v1.NdbCluster

	ginkgo.BeforeAll(func() {
		ginkgo.By("extracting values from TestContext")
		ns = tc.Namespace()
		c = tc.K8sClientset()
		// Create a new NdbCluster resource to be used by the test
		testNdb = testutils.NewTestNdb(ns, "upgrade-test", 2)

		ginkgo.DeferCleanup(func() {
			ginkgo.By("Delete the NdbCluster resource")
			ndbtest.KubectlDeleteNdbObj(c, testNdb)
		})
	})

	ginkgo.When("NdbCluster is created with a old MySQL Cluster image", func() {
		ginkgo.BeforeAll(func() {
			// Note : this image is preloaded into the K8s Cluster nodes by
			// the run-e2e-test.go tool to avoid any delays within this testcase.
			testNdb.Spec.Image = "container-registry.oracle.com/mysql/community-cluster:8.0.28"
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.It("should start all the MySQL Cluster nodes with the version specified in the image", func() {
			expectMySQLClusterVersion(c, testNdb)
		})
	})

	ginkgo.When("NdbCluster is updated with a newer MySQL Cluster image", func() {
		ginkgo.BeforeAll(func() {
			// Note : this image is preloaded into the K8s Cluster nodes by
			// the run-e2e-test.go tool to avoid any delays within this testcase.
			testNdb.Spec.Image = "container-registry.oracle.com/mysql/community-cluster:8.0.30"
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.It("should have upgraded all the MySQL Cluster nodes to the version specified in the image", func() {
			expectMySQLClusterVersion(c, testNdb)
		})
	})
})
