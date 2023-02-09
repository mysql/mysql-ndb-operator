// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"fmt"

	mysqlutils "github.com/mysql/ndb-operator/e2e-tests/utils/mysql"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	clientset "k8s.io/client-go/kubernetes"
)

func verifyMySQLCluster(clientset clientset.Interface, testNdbCluster *v1.NdbCluster) {
	// It should be enough to just check if the MySQL Server has
	// NDBCLUSTER support as the operator will send an 'SyncSuccess'
	// only after verifying the health of the MySQL Cluster nodes
	// and the pods they run in.
	ginkgo.By("Verifying that MySQL Server has NDBCLUSTER engine support", func() {
		db := mysqlutils.Connect(clientset, testNdbCluster, "")
		row := db.QueryRow("select support from information_schema.engines where engine = 'ndbcluster'")
		var value string
		ndbtest.ExpectNoError(row.Scan(&value), "select support from information_schema.engines failed")
		gomega.Expect(value).To(gomega.Equal("YES"),
			"MySQL Server does not have support for NDBCLUSTER engine.")
	})
}

var _ = ndbtest.NewTestCase("Multiple NDB Clusters maintained by a single NDB Operator", func(tc *ndbtest.TestContext) {
	var ns string
	var c clientset.Interface
	var numOfNdbClusters int
	var testNdbClusters []*v1.NdbCluster

	ginkgo.BeforeEach(func() {
		ginkgo.By("extracting values from TestContext")
		ns = tc.Namespace()
		c = tc.K8sClientset()
		numOfNdbClusters = 2
	})

	ginkgo.When("Multiple NdbCluster objects are created", func() {

		ginkgo.BeforeEach(func() {
			// Create the NdbCluster objects
			for i := 1; i <= numOfNdbClusters; i++ {
				testNdbCluster := testutils.NewTestNdb(ns, fmt.Sprintf("multiple-ndb-test-%d", i), 1)
				testNdbCluster.Spec.RedundancyLevel = 1
				testNdbClusters = append(testNdbClusters, testNdbCluster)

				// Create the NdbCluster Object in K8s and wait for the operator to deploy the MySQL Cluster
				ginkgo.By(
					fmt.Sprintf("Creating NdbCluster '%s/%s'", testNdbCluster.Namespace, testNdbCluster.Name),
					func() {
						ndbtest.KubectlApplyNdbObj(c, testNdbCluster)
					})
			}
		})

		ginkgo.AfterEach(func() {
			// Delete the NdbClusters
			for _, testNdbCluster := range testNdbClusters {
				ginkgo.By(
					fmt.Sprintf("Deleting NdbCluster '%s/%s'", testNdbCluster.Namespace, testNdbCluster.Name),
					func() {
						ndbtest.KubectlDeleteNdbObj(c, testNdbCluster)
					})
			}
		})

		ginkgo.It("should make multiple MySQL Clusters available in K8s Cluster", func() {
			// Verify the MySQL Clusters
			for _, testNdbCluster := range testNdbClusters {
				verifyMySQLCluster(c, testNdbCluster)
			}

		})
	})
})
