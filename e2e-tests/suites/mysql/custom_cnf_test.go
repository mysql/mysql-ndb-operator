// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"fmt"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	"github.com/mysql/ndb-operator/e2e-tests/utils/mgmapi"
	"github.com/mysql/ndb-operator/e2e-tests/utils/mysql"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbutils"
	"github.com/mysql/ndb-operator/e2e-tests/utils/secret"
)

func expectGlobalVariableValue(
	c clientset.Interface, testNdb *v1alpha1.NdbCluster,
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

var _ = ndbtest.NewTestCase("MySQL Custom cnf", func(tc *ndbtest.TestContext) {
	var ns string
	var c clientset.Interface
	var ndbName, mysqlRootSecretName string
	var testNdb *v1alpha1.NdbCluster

	ginkgo.BeforeEach(func() {
		ginkgo.By("extracting values from TestContext")
		ns = tc.Namespace()
		c = tc.K8sClientset()
	})

	ginkgo.When("a custom cnf property is specified for MySQL Server", func() {

		ginkgo.BeforeEach(func() {
			ndbName = "ndb-custom-cnf-test"
			mysqlRootSecretName = ndbName + "-root-secret"
			// create the secret first
			secretutils.CreateSecretForMySQLRootAccount(c, mysqlRootSecretName, ns)
			// create the Ndb resource
			testNdb = testutils.NewTestNdb(ns, ndbName, 2)
			testNdb.Spec.Mysqld.RootPasswordSecretName = mysqlRootSecretName
			testNdb.Spec.Mysqld.MyCnf = "[mysqld]\nmax-user-connections=42\nlog-bin=ON"
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.AfterEach(func() {
			// cleanup
			ndbtest.KubectlDeleteNdbObj(c, testNdb)
			// drop the secret
			secretutils.DeleteSecret(c, mysqlRootSecretName, ns)
		})

		ginkgo.It("should start the server with those values as the defaults", func() {
			// verify that max_user_connections is properly set in server
			expectGlobalVariableValue(c, testNdb, "max_user_connections", 42)

			// verify that the defaults doesn't override the value set by the operator
			expectGlobalVariableValue(c, testNdb, "log_bin", "OFF")

			// verify that the ndb_use_copying_alter_table variable has the default value
			expectGlobalVariableValue(c, testNdb, "ndb_use_copying_alter_table", "OFF")

			ginkgo.By("verifying that NdbCluster status was updated properly", func() {
				// expects the status.generatedRootPasswordSecretName to be empty
				// as spec.mysqld.rootPasswordSecretName is set
				ndbutils.ValidateNdbClusterStatus(tc.Ctx(), tc.NdbClientset(), ns, ndbName)
			})

			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 1)

			ginkgo.By("updating the my.cnf value", func() {
				testNdb.Spec.Mysqld.MyCnf =
					"[mysqld]\nmax-user-connections=42\nlog-bin=ON\nndb_use_copying_alter_table=ON\n"
				ndbtest.KubectlApplyNdbObj(c, testNdb)
			})

			// verify that the ndb_use_copying_alter_table variable has the new value
			expectGlobalVariableValue(c, testNdb, "ndb_use_copying_alter_table", "ON")

			// verify that the mgmd and data nodes still have the previous config version
			mgmapiutils.ExpectConfigVersionInMySQLClusterNodes(c, testNdb, 1)
		})
	})
})
