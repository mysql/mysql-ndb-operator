// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"

	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"

	mysqlutils "github.com/mysql/ndb-operator/e2e-tests/utils/mysql"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
)

const dbName = "test"

func getConfigMaps() []corev1.ConfigMap {
	return []corev1.ConfigMap{
		//configmap 1
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cm-with-key",
			},
			Data: map[string]string{
				"create": "CREATE DATABASE IF NOT EXISTS test;" +
					"USE test;" +
					"CREATE TABLE person (personId int,city varchar(255));",
				"insert": "USE test;" +
					"INSERT INTO person VALUES (1,'dindigul');",
				"update": "USE test;" +
					"UPDATE person SET personID = 2 WHERE city = 'dindigul';",
				"update2": "USE test;" +
					"UPDATE person SET personID = 3 WHERE city = 'dindigul';",
			},
		},
		//configmap 2
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cm-without-key",
			},
			Data: map[string]string{
				"car": "USE test;" +
					"CREATE TABLE car (carID int,city varchar(255));",
				"carage": "USE test;" +
					"CREATE TABLE carage (carID int,age int);",
				"purchase": "USE test;" +
					"CREATE TABLE purchase (carID int,purchase varchar(255));",
			},
		},
	}
}

// expectQueryIntValue will execute a given query in the database and check if the query output is same
// as the expected value
func expectQueryIntValue(
	ctx context.Context, c clientset.Interface, testNdb *v1.NdbCluster, query string, expectedValue int) {
	ginkgo.By("verifying given query has the right value")
	db := mysqlutils.Connect(c, testNdb, dbName)
	row := db.QueryRowContext(ctx, query)
	var value int
	ndbtest.ExpectNoError(row.Scan(&value),
		"error executing query: %s", query)
	gomega.Expect(value).To(gomega.Equal(expectedValue),
		"unexpected value returned from query: %s expected value: %q", query, expectedValue)
}

var _ = ndbtest.NewOrderedTestCase("Custom init scripts", func(tc *ndbtest.TestContext) {
	var c clientset.Interface
	var testNdb *v1.NdbCluster
	var ctx context.Context
	ginkgo.BeforeAll(func() {
		ginkgo.By("extracting values from TestContext")
		ctx = tc.Ctx()
		ns := tc.Namespace()
		c = tc.K8sClientset()

		// Create the NdbCluster object to be used by the testcases
		ndbName := "custom-init-script-test"
		testNdb = testutils.NewTestNdb(ns, ndbName, 2)
		testNdb.Spec.MysqlNode.InitScripts = map[string][]string{
			"cm-with-key": {
				"create", "insert", "update",
			},
			"cm-without-key": {},
		}

		// create Configmap
		cmArray := getConfigMaps()
		for _, cm := range cmArray {
			_, err := c.CoreV1().ConfigMaps(ns).Create(ctx, &cm, metav1.CreateOptions{})
			ndbtest.ExpectNoError(err, "failed to create ConfigMap %s", cm.Name)
			ginkgo.DeferCleanup(func(cmName string) {
				err := c.CoreV1().ConfigMaps(ns).Delete(ctx, cmName, metav1.DeleteOptions{})
				ndbtest.ExpectNoError(err, "failed to delete ConfigMap %s", cmName)
			}, cm.Name)
		}
		// create the NdbCluster
		ndbtest.KubectlApplyNdbObj(c, testNdb)
		ginkgo.DeferCleanup(func() {
			// delete the NdbCluster
			ndbtest.KubectlDeleteNdbObj(c, testNdb)
		})
	})
	ginkgo.When("initScripts is specified in NdbCluster spec", func() {
		ginkgo.It("should initialise the MySQL Server with the init scripts", func() {
			tableNameList := []string{"car", "carage", "purchase"}
			// check if all tables in tableNameList is present in database to see if all the scripts
			// in cm-without-key configmap is executed successfully.
			mysqlutils.ExpectTablesInDatabase(ctx, c, testNdb, tableNameList, dbName)
			// check the value of PersonID to see if all the scripts in cm-with-key configmap is
			// executed successfully.
			expectQueryIntValue(ctx, c, testNdb, "SELECT personID FROM person WHERE city = 'dindigul';", 2)
		})
	})
})
