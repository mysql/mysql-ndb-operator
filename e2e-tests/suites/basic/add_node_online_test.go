// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"

	mgmapiutils "github.com/mysql/ndb-operator/e2e-tests/utils/mgmapi"
	mysqlutils "github.com/mysql/ndb-operator/e2e-tests/utils/mysql"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	sfsetutils "github.com/mysql/ndb-operator/e2e-tests/utils/statefulset"

	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	clientset "k8s.io/client-go/kubernetes"
)

func verifyTableIsDistributedToAllDataNodes(
	ctx context.Context, c clientset.Interface,
	testNdb *v1.NdbCluster, dbName, tableName string) {

	db := mysqlutils.Connect(c, testNdb, "ndbinfo")
	query := "SELECT COUNT(DISTINCT current_primary) " +
		"FROM ndbinfo.dictionary_tables AS t, ndbinfo.table_fragments AS f " +
		"WHERE t.table_id = f.table_id AND t.database_name=? AND t.table_name=?"
	row := db.QueryRowContext(ctx, query, dbName, tableName)
	var distributedNodeCount int32
	ndbtest.ExpectNoError(row.Scan(&distributedNodeCount), "query %s failed", query)
	gomega.Expect(distributedNodeCount).To(gomega.Equal(testNdb.Spec.DataNode.NodeCount),
		"Table is not distributed to all nodes")
}

var _ = ndbtest.NewOrderedTestCase("Add Data Node Online", func(tc *ndbtest.TestContext) {
	var ns string
	var c clientset.Interface
	var testNdb *v1.NdbCluster
	var ctx context.Context

	ginkgo.BeforeAll(func() {
		ginkgo.By("extracting values from TestContext")
		ns = tc.Namespace()
		c = tc.K8sClientset()
		ctx = tc.Ctx()

		ginkgo.DeferCleanup(func() {
			ginkgo.By("Delete the NdbCluster resource")
			ndbtest.KubectlDeleteNdbObj(c, testNdb)
		})
	})

	ginkgo.When("a new NdbCluster is created", func() {
		ginkgo.BeforeAll(func() {
			// Create a new NdbCluster resource to be used by the test
			testNdb = testutils.NewTestNdb(ns, "add-node-online-test", 2)
			testNdb.Spec.MysqlNode.NodeCount = 1
		})

		ginkgo.It("should successfully deploy MySQL Cluster", func() {
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.It("should deploy the MySQL Cluster with initial Config Version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 1)
		})

		ginkgo.It("should start the expected number of data nodes", func() {
			sfsetutils.ExpectHasReplicas(c, ns, testNdb.GetWorkloadName(constants.NdbNodeTypeNdbmtd), 2)
		})

		ginkgo.It("should be able to connect to and run queries to create a database and table", func() {
			db := mysqlutils.Connect(c, testNdb, "")
			_, err := db.ExecContext(ctx, "create database test")
			ndbtest.ExpectNoError(err, "create database test failed")
			_, err = db.ExecContext(ctx, "create table test.t1 (id int, value char(10)) engine ndb")
			ndbtest.ExpectNoError(err, "create table t1 failed")
		})

		ginkgo.It("should distribute the table among all data nodes", func() {
			verifyTableIsDistributedToAllDataNodes(ctx, c, testNdb, "test", "t1")
		})
	})

	ginkgo.When("data nodes are scaled up", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.DataNode.NodeCount = 6
		})

		ginkgo.It("should successfully update MySQL Cluster", func() {
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.It("should update the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)
		})

		ginkgo.It("should start the new number of data nodes", func() {
			sfsetutils.ExpectHasReplicas(c, ns, testNdb.GetWorkloadName(constants.NdbNodeTypeNdbmtd), 6)
		})

		ginkgo.It("should be able to connect to and run queries to create a database and table", func() {
			db := mysqlutils.Connect(c, testNdb, "")
			_, err := db.ExecContext(ctx, "insert into test.t1 values (1, 'NDBCluster')")
			ndbtest.ExpectNoError(err)
		})

		ginkgo.It("should distribute the table among all data nodes, including the new ones", func() {
			verifyTableIsDistributedToAllDataNodes(ctx, c, testNdb, "test", "t1")
		})
	})

	// Do a non data node update to NdbCluster that will trigger a MySQL Cluster config update
	ginkgo.When("a non data node config is updated in NdbCluster", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.FreeAPISlots++
		})

		ginkgo.It("should successfully update MySQL Cluster", func() {
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.It("should update the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 3)
		})

		ginkgo.It("should maintain the number of data nodes", func() {
			sfsetutils.ExpectHasReplicas(c, ns, testNdb.GetWorkloadName(constants.NdbNodeTypeNdbmtd), 6)
		})

		ginkgo.It("should be able to connect to and run queries to create a database and table", func() {
			db := mysqlutils.Connect(c, testNdb, "")
			row := db.QueryRow("select value from test.t1 where id = 1")
			var value string
			ndbtest.ExpectNoError(row.Scan(&value), "select value from test.t1 failed")
			gomega.Expect(value).To(gomega.Equal("NDBCluster"),
				"'select' query returned unexpected value")
		})

		ginkgo.It("should distribute the table among all data nodes", func() {
			verifyTableIsDistributedToAllDataNodes(ctx, c, testNdb, "test", "t1")
		})
	})
})
