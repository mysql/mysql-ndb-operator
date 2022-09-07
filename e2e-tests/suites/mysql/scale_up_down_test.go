// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	mgmapiutils "github.com/mysql/ndb-operator/e2e-tests/utils/mgmapi"
	"github.com/mysql/ndb-operator/e2e-tests/utils/mysql"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbutils"
	"github.com/mysql/ndb-operator/e2e-tests/utils/secret"
	statefulsetutils "github.com/mysql/ndb-operator/e2e-tests/utils/statefulset"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = ndbtest.NewOrderedTestCase("MySQL Servers scaling up and down", func(tc *ndbtest.TestContext) {
	var ctx context.Context
	var ns, ndbName, mysqlRootSecretName string
	var c clientset.Interface
	var testNdb *v1alpha1.NdbCluster

	ginkgo.BeforeAll(func() {
		ginkgo.By("extracting values from TestContext")
		ctx = tc.Ctx()
		ns = tc.Namespace()
		c = tc.K8sClientset()

		ginkgo.Skip("Temporarily disable the testcase")

		// Create the NdbCluster object to be used by the testcases
		ndbName = "mysqld-scaling-test"
		testNdb = testutils.NewTestNdb(ns, ndbName, 2)

		// create the secret in K8s
		mysqlRootSecretName = ndbName + "-root-secret"
		secretutils.CreateSecretForMySQLRootAccount(ctx, c, mysqlRootSecretName, ns)

		// Setup cleanup methods
		ginkgo.DeferCleanup(func() {
			// common cleanup for all specs cleanup
			ndbtest.KubectlDeleteNdbObj(c, testNdb)
			// delete the secret
			secretutils.DeleteSecret(ctx, c, mysqlRootSecretName, ns)
		})
	})

	/*
		TestSpecs:

		1. Start MySQL Cluster with 0 MySQL Servers - to verify that this
		   doesn't affect the 'ensure phase' of the sync loop.
		2. Update a non-mysql spec - to confirm that the statefulset is not
		   affected by this; It should not be created yet.
		3. Scale up MySQL Servers to count that is less than the number of
		   FreeApiSlots - to verify that a new statefulset with required
		   replica is created and the MySQL Cluster config version has not
		   been updated.
		4. Scale down to 0 MySQL Servers - to verify that the statefulset
		   is deleted.
		5. Scale up the MySQL Servers again - to verify that the new
		   statefulset is not affected by the previous statefulset that
		   existed between step 3-4.
		6. Scale down the MySQL Servers to a non-zero replica - to verify
		   that the scale down is done right without reducing API slots.
		7. Scale up beyond the number of available free slots to verify
		   that more sections are added to the MySQL Cluster config via
		   a config update.
		#. Verify that MySQL queries run successfully whenever there is a
		   MySQL Server running.
		#. Verify that the data nodes and management nodes are not restarted
		   for scale up and scale down requests if free slots are available.
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
			ctx, tc.NdbClientset(), ns, ndbName)
	})

	// TestCase-1
	ginkgo.When("NdbCluster is created with nil MySQL Spec", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.Mysqld = nil
			testNdb.Spec.FreeAPISlots = 3
		})

		ginkgo.It("should not create a statefulset of MySQL Servers", func() {
			statefulsetutils.ExpectToBeNil(c, testNdb.Namespace, ndbName+"-mysqld")
		})

		ginkgo.It("should initialise the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 1)
		})
	})

	// TestCase-2
	ginkgo.When("a non MySQL Server spec is updated", func() {
		ginkgo.BeforeAll(func() {
			dataMemory := intstr.FromString("150M")
			testNdb.Spec.DataNode.Config = map[string]*intstr.IntOrString{
				"DataMemory": &dataMemory,
			}
		})

		ginkgo.It("should still not create a statefulset of MySQL Servers", func() {
			statefulsetutils.ExpectToBeNil(c, testNdb.Namespace, ndbName+"-mysqld")
		})

		ginkgo.It("should increment the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)
		})
	})

	// TestCase-3
	ginkgo.When("MySQL Server are scaled up to a count less than the FreeApiSlots", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.Mysqld = &v1alpha1.NdbMysqldSpec{
				RootPasswordSecretName: mysqlRootSecretName,
				NodeCount:              2,
			}
		})

		ginkgo.It("should create a statefulset of MySQL Servers with given replica", func() {
			statefulsetutils.ExpectHasReplicas(c, testNdb.Namespace, ndbName+"-mysqld", 2)
		})

		ginkgo.It("should not update the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)
		})

		ginkgo.It("should be able to connect to and run queries to create a database and table", func() {
			db := mysqlutils.Connect(c, testNdb, "")
			_, err := db.Exec("create database test")
			ndbtest.ExpectNoError(err, "create database test failed")
			_, err = db.Exec("create table test.t1 (id int, value char(10)) engine ndb")
			ndbtest.ExpectNoError(err, "create table t1 failed")
		})
	})

	// TestCase-4
	ginkgo.When("MySQL Servers are scaled down to 0", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.Mysqld = nil
		})

		ginkgo.It("should delete the statefulset of MySQL Servers", func() {
			statefulsetutils.ExpectToBeNil(c, testNdb.Namespace, ndbName+"-mysqld")
		})

		ginkgo.It("should not update the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)
		})
	})

	// TestCase-5
	ginkgo.When("MySQL Servers are scaled up from 0", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.Mysqld = &v1alpha1.NdbMysqldSpec{
				NodeCount:              3,
				RootPasswordSecretName: mysqlRootSecretName,
			}
		})

		ginkgo.It("should create a statefulset of MySQL Servers with given replica", func() {
			statefulsetutils.ExpectHasReplicas(c, testNdb.Namespace, ndbName+"-mysqld", 3)
		})

		ginkgo.It("should not update the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)
		})

		ginkgo.It("should be able to run queries to insert into table", func() {
			db := mysqlutils.Connect(c, testNdb, "test")
			result, err := db.Exec("insert into t1 values (1, 'ndb'), (2, 'operator')")
			ndbtest.ExpectNoError(err, "insert into t1 failed")
			gomega.Expect(result.RowsAffected()).To(gomega.Equal(int64(2)))
		})
	})

	// TestCase-6
	ginkgo.When("MySQL Servers are scaled down to a non zero count", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.Mysqld.NodeCount = 1
		})

		ginkgo.It("should scale down the statefulset of MySQL Servers", func() {
			statefulsetutils.ExpectHasReplicas(c, testNdb.Namespace, ndbName+"-mysqld", 1)
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

	// TestCase-7
	ginkgo.When("MySQL Servers are scaled up beyond the number of available FreeApiSlots", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.Mysqld.NodeCount = 5
		})

		ginkgo.It("should scale-up the statefulset of MySQL Servers", func() {
			statefulsetutils.ExpectHasReplicas(c, testNdb.Namespace, ndbName+"-mysqld", 5)
		})

		ginkgo.It("should increment the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 3)
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
