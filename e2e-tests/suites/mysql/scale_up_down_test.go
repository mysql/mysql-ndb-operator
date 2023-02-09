// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"

	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	mgmapiutils "github.com/mysql/ndb-operator/e2e-tests/utils/mgmapi"
	mysqlutils "github.com/mysql/ndb-operator/e2e-tests/utils/mysql"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbutils"
	statefulsetutils "github.com/mysql/ndb-operator/e2e-tests/utils/statefulset"

	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = ndbtest.NewOrderedTestCase("MySQL Servers scaling up and down", func(tc *ndbtest.TestContext) {
	var ctx context.Context
	var ns, ndbName string
	var c clientset.Interface
	var testNdb *v1.NdbCluster
	var mysqlSfsetName string

	ginkgo.BeforeAll(func() {
		ginkgo.By("extracting values from TestContext")
		ctx = tc.Ctx()
		ns = tc.Namespace()
		c = tc.K8sClientset()

		// Create the NdbCluster object to be used by the testcases
		ndbName = "mysqld-scaling-test"
		testNdb = testutils.NewTestNdb(ns, ndbName, 2)

		mysqlSfsetName = testNdb.GetWorkloadName(constants.NdbNodeTypeMySQLD)

		// Setup cleanup methods
		ginkgo.DeferCleanup(func() {
			// common cleanup for all specs cleanup
			ndbtest.KubectlDeleteNdbObj(c, testNdb)
		})
	})

	/*
		TestSpecs:

		1. Start MySQL Cluster with 0 MySQL Servers - to verify that this
		   brings up 1 MySQL Server by default
		2. Scale up MySQL Servers and increment MaxNodeCount - to verify
		   there is a MySQL Cluster config update.
		3. Scale up MySQL Servers to count that is less than MaxNodeCount
		   - to verify that the MySQL Cluster config version has not
		   been updated.
		4. Scale down to 2 MySQL Servers - to verify that the MySQL Cluster
		   config is not updated.
		#. Verify that MySQL queries run successfully whenever there is a
		   MySQL Server running.
	*/

	// Use a OncePerOrdered JustBeforeEach to create/modify the
	// NdbCluster in K8s Server and validate the status.
	// Note : This JustBeforeEach only applies the NdbCluster object
	// to K8s Cluster. The actual changes to the object are made by
	// the BeforeAlls inside the Whens based on the spec requirements.
	ginkgo.JustBeforeEach(ginkgo.OncePerOrdered, func() {
		ndbtest.KubectlApplyNdbObjNoWait(testNdb)
		// validate the status updates made by the operator during the sync
		ndbutils.ValidateNdbClusterStatusUpdatesDuringSync(
			ctx, tc.NdbClientset(), ns, ndbName, false)
	})

	// TestCase-1
	ginkgo.When("NdbCluster is created with nil MySQL Spec", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.MysqlNode = nil
		})

		ginkgo.It("should create a statefulset of MySQL Servers", func() {
			statefulsetutils.ExpectHasReplicas(c, ns, mysqlSfsetName, 1)
		})

		ginkgo.It("should initialise the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 1)
		})

		ginkgo.It("should be able to connect to and run queries to create a database and table", func() {
			// Update testNdb to reflect the changes made by the mutator
			testNdb.Spec.MysqlNode = &v1.NdbMysqldSpec{
				NodeCount:    1,
				MaxNodeCount: 1,
			}
			db := mysqlutils.Connect(c, testNdb, "")
			_, err := db.Exec("create database test")
			ndbtest.ExpectNoError(err, "create database test failed")
			_, err = db.Exec("create table test.t1 (id int, value char(10)) engine ndb")
			ndbtest.ExpectNoError(err, "create table t1 failed")
		})
	})

	// TestCase-2
	ginkgo.When("NodeCount and MaxNodeCount are increased", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.MysqlNode = &v1.NdbMysqldSpec{
				NodeCount:    3,
				MaxNodeCount: 5,
			}
		})

		ginkgo.It("should scale up the MySQL Servers count", func() {
			statefulsetutils.ExpectHasReplicas(c, ns, mysqlSfsetName, 3)
		})

		ginkgo.It("should increment the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)
		})

		ginkgo.It("should be able to run queries to insert into table", func() {
			db := mysqlutils.Connect(c, testNdb, "test")
			result, err := db.Exec("insert into t1 values (1, 'ndb'), (2, 'operator')")
			ndbtest.ExpectNoError(err, "insert into t1 failed")
			gomega.Expect(result.RowsAffected()).To(gomega.Equal(int64(2)))
		})
	})

	// TestCase-3
	ginkgo.When("NodeCount is increased to a value less than MaxNodeCount", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.MysqlNode.NodeCount = 4
		})

		ginkgo.It("should scale up the MySQL Servers", func() {
			statefulsetutils.ExpectHasReplicas(c, ns, mysqlSfsetName, 4)
		})

		ginkgo.It("should not update the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)
		})

		ginkgo.It("should be able to run queries to read from table", func() {
			db := mysqlutils.Connect(c, testNdb, "test")
			row := db.QueryRow("select value from t1 where id = 1")
			var value string
			ndbtest.ExpectNoError(row.Scan(&value), "select value from t1 failed")
			gomega.Expect(value).To(gomega.Equal("ndb"),
				"'select' query returned unexpected value")
		})
	})

	// TestCase-4
	ginkgo.When("NodeCount is decreased to 2", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.MysqlNode.NodeCount = 2
		})

		ginkgo.It("should scale down the MySQL Servers", func() {
			statefulsetutils.ExpectHasReplicas(c, ns, mysqlSfsetName, 2)
		})

		ginkgo.It("should not update the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)
		})

		ginkgo.It("should be able to run queries to read from table", func() {
			db := mysqlutils.Connect(c, testNdb, "test")
			row := db.QueryRow("select value from t1 where id = 2")
			var value string
			ndbtest.ExpectNoError(row.Scan(&value), "select value from t1 failed")
			gomega.Expect(value).To(gomega.Equal("operator"),
				"'select' query returned unexpected value")
		})
	})
})
