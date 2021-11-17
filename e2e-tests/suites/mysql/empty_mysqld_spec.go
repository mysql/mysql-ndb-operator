// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"github.com/mysql/ndb-operator/e2e-tests/utils/deployment"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	"github.com/onsi/ginkgo"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = ndbtest.NewTestCase("Setting/Resetting MySQL Spec to nil", func(tc *ndbtest.TestContext) {
	var ns, ndbName string
	var c clientset.Interface
	var testNdb *v1alpha1.NdbCluster

	ginkgo.BeforeEach(func() {
		ginkgo.By("extracting values from TestContext")
		ns = tc.Namespace()
		c = tc.K8sClientset()
	})

	ginkgo.AfterEach(func() {
		// common cleanup for all specs cleanup
		ndbtest.KubectlDeleteNdbObj(c, testNdb)
	})

	ginkgo.When("MySQL Cluster is started with 0 MySQL Servers", func() {

		/*
			1. Start MySQL Cluster with 0 MySQL Servers - to verify that this
			   doesn't affect the 'ensure phase' of the sync loop.
			2. Update a non-mysql spec - to confirm that the deployment is not
			   affected by this; It should not be created yet.
			3. Scale up MySQL Servers - to verify that a new deployment with
			   required replica is created.
			4. Scale down to 0 MySQL Servers - to verify that the deployment
			   is deleted.
			5. Scale up the MySQL Servers again - to verify that the new
			   deployment is not affected by the previous deployment that
			   existed between step 3-4.
		*/

		ginkgo.It("should scale up and down properly", func() {

			ginkgo.By("starting a MySQL Cluster with 0 MySQL Servers", func() {
				// create the Ndb resource
				ndbName = "ndb-mysqld-test"
				testNdb = testutils.NewTestNdb(ns, ndbName, 2)
				testNdb.Spec.Mysqld = nil
				ndbtest.KubectlApplyNdbObj(c, testNdb)
			})

			ginkgo.By("verifying that deployment doesn't exist", func() {
				deployment.ExpectToBeNil(c, testNdb.Namespace, ndbName+"-mysqld")
			})

			ginkgo.By("changing a non MySQL Server spec", func() {
				testNdb.Spec.DataMemory = "150M"
				ndbtest.KubectlApplyNdbObj(c, testNdb)
			})

			ginkgo.By("verifying that the deployment still doesn't exist", func() {
				deployment.ExpectToBeNil(c, testNdb.Namespace, ndbName+"-mysqld")
			})

			ginkgo.By("scaling up the MySQL Servers", func() {
				testNdb.Spec.Mysqld = &v1alpha1.NdbMysqldSpec{
					NodeCount: 2,
				}
				ndbtest.KubectlApplyNdbObj(c, testNdb)
			})

			ginkgo.By("verifying the number of MySQL Servers running after scale up", func() {
				deployment.ExpectHasReplicas(c, testNdb.Namespace, ndbName+"-mysqld", 2)
			})

			ginkgo.By("scaling down the MySQL Servers to 0", func() {
				testNdb.Spec.Mysqld = nil
				ndbtest.KubectlApplyNdbObj(c, testNdb)
			})

			ginkgo.By("verifying that deployment doesn't exist", func() {
				deployment.ExpectToBeNil(c, testNdb.Namespace, ndbName+"-mysqld")
			})

			ginkgo.By("increasing the number of servers from 0", func() {
				testNdb.Spec.Mysqld = &v1alpha1.NdbMysqldSpec{
					NodeCount: 3,
				}
				ndbtest.KubectlApplyNdbObj(c, testNdb)
			})

			ginkgo.By("verifying expected number of MySQL Servers are running", func() {
				deployment.ExpectHasReplicas(c, testNdb.Namespace, ndbName+"-mysqld", 3)
			})

		})
	})
})
