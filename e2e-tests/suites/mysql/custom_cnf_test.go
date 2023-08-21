// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"fmt"

	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	clientset "k8s.io/client-go/kubernetes"

	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	mgmapiutils "github.com/mysql/ndb-operator/e2e-tests/utils/mgmapi"
	mysqlutils "github.com/mysql/ndb-operator/e2e-tests/utils/mysql"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	secretutils "github.com/mysql/ndb-operator/e2e-tests/utils/secret"
)

func expectGlobalVariableValue(
	c clientset.Interface, testNdb *v1.NdbCluster,
	variableName string, expectedValue interface{}) {
	ginkgo.By(fmt.Sprintf("verifying %q has the right value", variableName))
	db := mysqlutils.Connect(c, testNdb, "performance_schema")
	row := db.QueryRow(
		"select variable_value from global_variables where variable_name = ?", variableName)
	var value string
	ndbtest.ExpectNoError(row.Scan(&value),
		"querying for %q returned an error", variableName)
	gomega.Expect(value).To(gomega.BeEquivalentTo(fmt.Sprint(expectedValue)),
		"%q had an unexpected value", variableName)
}

var _ = ndbtest.NewOrderedTestCase("MySQL Custom cnf", func(tc *ndbtest.TestContext) {
	var c clientset.Interface
	var testNdb *v1.NdbCluster
	var myCnf map[string]string

	ginkgo.BeforeAll(func() {
		ginkgo.By("extracting values from TestContext")
		ctx := tc.Ctx()
		ns := tc.Namespace()
		c = tc.K8sClientset()
		myCnf = make(map[string]string)

		// Create the NdbCluster object to be used by the testcases
		ndbName := "ndb-custom-cnf-test"
		testNdb = testutils.NewTestNdb(ns, ndbName, 2)

		// create the secret in K8s
		mysqlRootSecretName := ndbName + "-root-secret"
		ginkgo.By("creating MySQL root account secret")
		secretutils.CreateSecret(ctx, c, mysqlRootSecretName, ns)
		testNdb.Spec.MysqlNode.RootPasswordSecretName = mysqlRootSecretName

		// Setup cleanup methods
		ginkgo.DeferCleanup(func() {
			// common cleanup for all specs cleanup
			ndbtest.KubectlDeleteNdbObj(c, testNdb)
			// delete the secret
			secretutils.DeleteSecret(ctx, c, mysqlRootSecretName, ns)
		})
	})

	// Use a OncePerOrdered JustBeforeEach to create/modify the
	// NdbCluster in K8s Server.
	ginkgo.JustBeforeEach(ginkgo.OncePerOrdered, func() {
		// Generate my.cnf value
		myCnfStr := "[mysqld]\n"
		for key, value := range myCnf {
			myCnfStr += key + "=" + value + "\n"
		}
		// Set the my.cnf value and create/update the NdbCluster
		testNdb.Spec.MysqlNode.MyCnf = myCnfStr
		ndbtest.KubectlApplyNdbObj(c, testNdb)
	})

	ginkgo.When("a custom my.cnf is specified in NdbCluster", func() {
		ginkgo.BeforeAll(func() {
			myCnf["max-user-connections"] = "42"
		})

		ginkgo.It("should start MySQL Servers with the given configuration", func() {
			// verify that max_user_connections is properly set in server
			expectGlobalVariableValue(c, testNdb, "max_user_connections", 42)

			// verify that the ndb_use_copying_alter_table variable has the default value
			expectGlobalVariableValue(c, testNdb, "ndb_use_copying_alter_table", "OFF")
		})

		ginkgo.It("should initialise the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 1)
		})
	})

	ginkgo.When("the custom my.cnf is updated", func() {
		ginkgo.BeforeAll(func() {
			myCnf["ndb_use_copying_alter_table"] = "ON"
		})

		ginkgo.It("should update the MySQL Servers with the new config", func() {
			// verify that the ndb_use_copying_alter_table variable has the new value
			expectGlobalVariableValue(c, testNdb, "ndb_use_copying_alter_table", "ON")

			// verify that the other config values are preserved
			expectGlobalVariableValue(c, testNdb, "max_user_connections", 42)
		})

		ginkgo.It("should not update the MySQL Cluster config version", func() {
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 1)
		})
	})
})
