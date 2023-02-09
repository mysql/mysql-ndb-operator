// Copyright (c) 2022, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"
	"fmt"

	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	mgmapiutils "github.com/mysql/ndb-operator/e2e-tests/utils/mgmapi"
	mysqlutils "github.com/mysql/ndb-operator/e2e-tests/utils/mysql"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	statefulsetutils "github.com/mysql/ndb-operator/e2e-tests/utils/statefulset"
)

func verifyMySQLClusterAPIConfig(ctx context.Context, c clientset.Interface, testNdb *v1.NdbCluster) {
	db := mysqlutils.Connect(c, testNdb, "ndbinfo")
	hostnamePattern := fmt.Sprintf("%s-mysqld-%%.%s.%s.svc.%%", testNdb.Name, testNdb.Name, testNdb.Namespace)
	rows, err := db.QueryContext(ctx,
		"SELECT * FROM config_nodes WHERE node_hostname LIKE ? ORDER BY node_id", hostnamePattern)
	ndbtest.ExpectNoError(err, "SELECT failed")
	defer ndbtest.ExpectNoError(rows.Close())

	// Extract the results and verify
	var nodeType, hostname string
	var nodeId int
	expectedPodIdx := 0
	currPoolSize := 0
	expectedNodeId := constants.NdbNodeTypeAPIStartNodeId
	for rows.Next() {
		ndbtest.ExpectNoError(rows.Scan(&nodeId, &nodeType, &hostname), "scan failed")
		gomega.Expect(nodeType).To(gomega.Equal("API"))
		gomega.Expect(hostname).To(gomega.MatchRegexp("%s-%d\\.%s\\.%s.*", testNdb.Name, expectedPodIdx, testNdb.Name, testNdb.Namespace))
		gomega.Expect(nodeId).To(gomega.Equal(expectedNodeId))
		expectedNodeId++
		currPoolSize++
		if currPoolSize == int(testNdb.GetMySQLServerConnectionPoolSize()) {
			currPoolSize = 0
			expectedPodIdx++
		}
	}
}

var _ = ndbtest.NewOrderedTestCase("NDB Connection pool", func(tc *ndbtest.TestContext) {
	var ctx context.Context
	var c clientset.Interface
	var testNdb *v1.NdbCluster
	var ns, mysqlSfsetName string

	ginkgo.BeforeAll(func() {
		ginkgo.By("extracting values from TestContext")
		ctx = tc.Ctx()
		ns = tc.Namespace()
		c = tc.K8sClientset()

		// Create the NdbCluster object to be used by the testcases
		ndbName := "ndb-connection-pool-test"
		testNdb = testutils.NewTestNdb(ns, ndbName, 2)

		mysqlSfsetName = testNdb.GetWorkloadName(constants.NdbNodeTypeMySQLD)

		// Setup cleanup methods
		ginkgo.DeferCleanup(func() {
			// common cleanup for all specs cleanup
			ndbtest.KubectlDeleteNdbObj(c, testNdb)
		})
	})

	ginkgo.When("connection pool is specified in CRD spec", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.MysqlNode.MaxNodeCount = 5
			testNdb.Spec.MysqlNode.ConnectionPoolSize = 2
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.It("should declare and reserve expected number of API sections", func() {
			verifyMySQLClusterAPIConfig(ctx, c, testNdb)
		})

		ginkgo.It("should deploy specified number of MySQL Servers", func() {
			statefulsetutils.ExpectHasReplicas(c, ns, mysqlSfsetName, 2)
		})

		ginkgo.It("should initialise MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 1)
		})
	})

	ginkgo.When("MySQL Server nodeCount is increased", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.MysqlNode.NodeCount = 4
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.It("should scale up the MySQL Servers", func() {
			statefulsetutils.ExpectHasReplicas(c, ns, mysqlSfsetName, 4)
		})

		ginkgo.It("should not update the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 1)
		})
	})

	ginkgo.When("MySQL Server connectionPoolSize is increased", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.MysqlNode.ConnectionPoolSize = 3
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.It("should increase the MySQL API sections", func() {
			verifyMySQLClusterAPIConfig(ctx, c, testNdb)
		})

		ginkgo.It("should still deploy same number of MySQL Servers", func() {
			statefulsetutils.ExpectHasReplicas(c, ns, mysqlSfsetName, 4)
		})

		ginkgo.It("should update the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 2)
		})
	})
})
