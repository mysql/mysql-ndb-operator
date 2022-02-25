// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"

	"github.com/mysql/ndb-operator/e2e-tests/utils/deployment"
	"github.com/mysql/ndb-operator/e2e-tests/utils/mgmapi"
	"github.com/mysql/ndb-operator/e2e-tests/utils/mysql"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbutils"
	"github.com/mysql/ndb-operator/e2e-tests/utils/secret"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	ndbclientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = ndbtest.NewTestCase("MySQL Servers scaling up and down", func(tc *ndbtest.TestContext) {
	var ctx context.Context
	var ns, ndbName, mysqlRootSecretName string
	var c clientset.Interface
	var ndbClient ndbclientset.Interface
	var testNdb *v1alpha1.NdbCluster

	ginkgo.BeforeEach(func() {
		ginkgo.By("extracting values from TestContext")
		ctx = tc.Ctx()
		ns = tc.Namespace()
		c = tc.K8sClientset()
		ndbClient = tc.NdbClientset()

		ndbName = "ndb-custom-cnf-test"
		mysqlRootSecretName = ndbName + "-root-secret"
		// create the secret first
		secretutils.CreateSecretForMySQLRootAccount(c, mysqlRootSecretName, ns)

	})

	ginkgo.AfterEach(func() {
		// common cleanup for all specs cleanup
		ndbtest.KubectlDeleteNdbObj(c, testNdb)
		// drop the secret
		secretutils.DeleteSecret(c, mysqlRootSecretName, ns)
	})

	ginkgo.When("NdbCluster mysqld spec nodeCount is updated", func() {

		/*
			TestScenarios:

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
			6. Scale down the MySQL Servers to a non-zero replica - to verify
			   that the scale down is done right without reducing API slots.
			7. Scale up beyond the number of available free slots to verify
			   that more sections are added to the MySQL Cluster config via
			   a config update.
			#. Verify that MySQL queries run successfully whenever there is a
			   MySQL Server running.
			#. Verify that the data nodes and management nodes are not restarted
			   for scale up requests if free slots are available.
		*/

		ginkgo.It("should scale up and down properly", func() {

			// TestCase-1
			ginkgo.By("starting a MySQL Cluster with 0 MySQL Servers", func() {
				// create the Ndb resource
				ndbName = "mysqld-empty-spec-test"
				testNdb = testutils.NewTestNdb(ns, ndbName, 2)
				testNdb.Spec.Mysqld = nil
				testNdb.Spec.FreeAPISlots = 3
				ndbtest.KubectlApplyNdbObjNoWait(testNdb)
				// validate the status updates made by the operator during the sync
				ndbutils.ValidateNdbClusterStatusUpdatesDuringSync(
					ctx, ndbClient, ns, ndbName, true)
			})

			ginkgo.By("verifying that deployment doesn't exist", func() {
				deployment.ExpectToBeNil(c, testNdb.Namespace, ndbName+"-mysqld")
			})

			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 1)

			// TestCase-2
			ginkgo.By("changing a non MySQL Server spec", func() {
				testNdb.Spec.DataMemory = "150M"
				ndbtest.KubectlApplyNdbObjNoWait(testNdb)
				// validate the status updates made by the operator during the sync
				ndbutils.ValidateNdbClusterStatusUpdatesDuringSync(
					ctx, ndbClient, ns, ndbName, false)
			})

			ginkgo.By("verifying that the deployment still doesn't exist", func() {
				deployment.ExpectToBeNil(c, testNdb.Namespace, ndbName+"-mysqld")
			})

			// expect an increment in data/mgmd config version
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)

			// TestCase-3
			ginkgo.By("scaling up the MySQL Servers", func() {
				testNdb.Spec.Mysqld = &v1alpha1.NdbMysqldSpec{
					RootPasswordSecretName: mysqlRootSecretName,
					NodeCount:              2,
				}
				ndbtest.KubectlApplyNdbObjNoWait(testNdb)
				// validate the status updates made by the operator during the sync
				ndbutils.ValidateNdbClusterStatusUpdatesDuringSync(
					ctx, ndbClient, ns, ndbName, false)
			})

			ginkgo.By("verifying the number of MySQL Servers running after scale up", func() {
				deployment.ExpectHasReplicas(c, testNdb.Namespace, ndbName+"-mysqld", 2)
			})

			// data/mgmd config version should not change
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)

			ginkgo.By("running queries to create a database and table", func() {
				db := mysqlutils.Connect(c, testNdb, "")
				_, err := db.Exec("create database test")
				ndbtest.ExpectNoError(err, "create database test failed")
				_, err = db.Exec("create table test.t1 (id int, value char(10)) engine ndb")
				ndbtest.ExpectNoError(err, "create table t1 failed")
			})

			// TestCase-4
			ginkgo.By("scaling down the MySQL Servers to 0", func() {
				testNdb.Spec.Mysqld = nil
				ndbtest.KubectlApplyNdbObjNoWait(testNdb)
				// validate the status updates made by the operator during the sync
				ndbutils.ValidateNdbClusterStatusUpdatesDuringSync(
					ctx, ndbClient, ns, ndbName, false)
			})

			ginkgo.By("verifying that deployment doesn't exist", func() {
				deployment.ExpectToBeNil(c, testNdb.Namespace, ndbName+"-mysqld")
			})

			// data/mgmd config version should not change
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)

			// TestCase-5
			ginkgo.By("scaling up the MySQL servers from 0", func() {
				testNdb.Spec.Mysqld = &v1alpha1.NdbMysqldSpec{
					NodeCount:              3,
					RootPasswordSecretName: mysqlRootSecretName,
				}
				ndbtest.KubectlApplyNdbObjNoWait(testNdb)
				// validate the status updates made by the operator during the sync
				ndbutils.ValidateNdbClusterStatusUpdatesDuringSync(
					ctx, ndbClient, ns, ndbName, false)
			})

			ginkgo.By("verifying expected number of MySQL Servers are running", func() {
				deployment.ExpectHasReplicas(c, testNdb.Namespace, ndbName+"-mysqld", 3)
			})

			// data/mgmd config version should not change
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)

			ginkgo.By("running queries to insert into t1", func() {
				db := mysqlutils.Connect(c, testNdb, "test")
				result, err := db.Exec("insert into t1 values (1, 'ndb'), (2, 'operator')")
				ndbtest.ExpectNoError(err, "insert into t1 failed")
				gomega.Expect(result.RowsAffected()).To(gomega.Equal(int64(2)))
			})

			// TestCase-6
			ginkgo.By("scaling down the MySQL Servers", func() {
				testNdb.Spec.Mysqld.NodeCount = 1
				ndbtest.KubectlApplyNdbObj(c, testNdb)
			})

			ginkgo.By("verifying the MySQL Server node count after scale down", func() {
				deployment.ExpectHasReplicas(c, testNdb.Namespace, ndbName+"-mysqld", 1)
			})

			// data/mgmd config version should not change
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)

			ginkgo.By("running queries to read from t1", func() {
				db := mysqlutils.Connect(c, testNdb, "test")
				row := db.QueryRow("select value from t1 where id = 1")
				var value string
				ndbtest.ExpectNoError(row.Scan(&value), "select value from t1 failed")
				gomega.Expect(value).To(gomega.Equal("ndb"),
					"'select' query returned unexpected value")
			})

			// Testcase-7
			ginkgo.By("scaling up the MySQL Servers beyond the available freeAPISlots", func() {
				testNdb.Spec.Mysqld.NodeCount = 5
				ndbtest.KubectlApplyNdbObj(c, testNdb)
			})

			ginkgo.By("verifying expected number of MySQL Servers are running after scale-up", func() {
				deployment.ExpectHasReplicas(c, testNdb.Namespace, ndbName+"-mysqld", 5)
			})

			// data/mgmd config version should change as the config had to be updated with more API sections
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 3)

			ginkgo.By("running queries to read from t1", func() {
				db := mysqlutils.Connect(c, testNdb, "test")
				row := db.QueryRow("select value from t1 where id = 2")
				var value string
				ndbtest.ExpectNoError(row.Scan(&value), "select value from t1 failed")
				gomega.Expect(value).To(gomega.Equal("operator"),
					"'select' query returned unexpected value")
			})

		})
	})
})
