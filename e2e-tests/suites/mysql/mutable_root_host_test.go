// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/mysql/ndb-operator/e2e-tests/utils/k8sutils"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbutils"
	secretutils "github.com/mysql/ndb-operator/e2e-tests/utils/secret"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
)

// Helper function to build a set of bash commands that is executed in pod/exec
func buildExecCmd(ctx context.Context, c clientset.Interface, testNdb *v1alpha1.NdbCluster, ns string) []string {
	//extract password from secret
	password := secretutils.GetMySQLRootPassword(ctx, c, testNdb)
	password = "-p" + password
	ndbName := testNdb.Name

	//extract hostname from mysql pod
	Selector := labels.Set{constants.ClusterNodeTypeLabel: "mysqld", constants.ClusterLabel: ndbName}.AsSelector()
	mysqlpodlist, _ := c.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: Selector.String(),
	})
	hostname := "-h" + mysqlpodlist.Items[1].Status.PodIP

	//Build query string
	query := "-e\"use mysql;select user, host from mysql.user;\""

	cmd := []string{
		"sh",
		"-c",
		"mysql -uroot " + password + " " + hostname + " " + query,
	}
	return cmd
}

var _ = ndbtest.NewOrderedTestCase("Mutable root host", func(tc *ndbtest.TestContext) {
	var ns string
	var c clientset.Interface
	var testNdb *v1alpha1.NdbCluster
	var ctx context.Context
	var mysqlpodlist *v1.PodList
	var ndbName string

	ginkgo.BeforeAll(func() {
		ginkgo.By("extracting values from TestContext")
		ns = tc.Namespace()
		c = tc.K8sClientset()
		ctx = tc.Ctx()

		// Create the NdbCluster object to be used by the testcases
		ndbName = "mutable-roothost-test"
		testNdb = testutils.NewTestNdb(ns, ndbName, 2)

		// Setup cleanup methods
		ginkgo.DeferCleanup(func() {
			// common cleanup for all specs cleanup
			ndbtest.KubectlDeleteNdbObj(c, testNdb)
		})

	})

	ginkgo.JustBeforeEach(ginkgo.OncePerOrdered, func() {
		ndbtest.KubectlApplyNdbObjNoWait(testNdb)
		// validate the status updates made by the operator during the sync
		ndbutils.ValidateNdbClusterStatusUpdatesDuringSync(
			ctx, tc.NdbClientset(), ns, ndbName)

		Selector := labels.Set{constants.ClusterNodeTypeLabel: "mysqld", constants.ClusterLabel: ndbName}.AsSelector()
		if mysqlpodlist == nil {
			mysqlpodlist, _ = c.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
				LabelSelector: Selector.String(),
			})
		}
	})

	// Due to Bug #33904288: host change from % -> any address is not getting distributed
	// So, testing from Unknown ip -> validip -> %

	//CASE 1: RootHost = Unknown ip
	ginkgo.When("Root host in MySQL Server is assigned to ip outside cluster", func() {

		ginkgo.BeforeAll(func() {
			testNdb.Spec.Mysqld.RootHost = "1.1.0.1"
		})

		ginkgo.It("ensure that root user is not accessible from any of the mysql pod in cluster", func() {
			cmd := buildExecCmd(ctx, c, testNdb, ns)

			_, _, err := k8sutils.Exec(c, mysqlpodlist.Items[0].Name, ns, cmd)
			ndbtest.ExpectError(err, "Expecting failure but succeed")

			_, _, err = k8sutils.Exec(c, mysqlpodlist.Items[1].Name, ns, cmd)
			ndbtest.ExpectError(err, "Expecting failure but succeed")
		})
	})

	//CASE 2: RootHost = valid ip
	ginkgo.When("Root host in MySQL Server is assigned to ip of one of the mysql pod in cluster", func() {

		ginkgo.BeforeAll(func() {
			testNdb.Spec.Mysqld.RootHost = mysqlpodlist.Items[0].Status.PodIP
		})

		ginkgo.It("ensure that root user is accessible from only one pod", func() {
			cmd := buildExecCmd(ctx, c, testNdb, ns)

			_, _, err := k8sutils.Exec(c, mysqlpodlist.Items[0].Name, ns, cmd)
			ndbtest.ExpectNoError(err, "Failure executing command %s", cmd)

			_, _, err = k8sutils.Exec(c, mysqlpodlist.Items[1].Name, ns, cmd)
			ndbtest.ExpectError(err, "Expecting failure but succeed")
		})
	})

	//CASE 3: RootHost = %
	ginkgo.When("Root host in MySQL Server is set to %", func() {

		ginkgo.BeforeAll(func() {
			testNdb.Spec.Mysqld.RootHost = "%"
		})

		ginkgo.It("ensure that root user is accessible from all pod", func() {
			cmd := buildExecCmd(ctx, c, testNdb, ns)

			_, _, err := k8sutils.Exec(c, mysqlpodlist.Items[0].Name, ns, cmd)
			ndbtest.ExpectNoError(err, "Failure executing command %s", cmd)

			_, _, err = k8sutils.Exec(c, mysqlpodlist.Items[1].Name, ns, cmd)
			ndbtest.ExpectNoError(err, "Failure executing command %s", cmd)
		})

	})

	//CASE 4: Scaledown and Scaleup mysqld with RootHost = unknown ip
	ginkgo.When("MySQL Servers are scaled down to 0 and scaled up again with RootHost = unknown ip", func() {

		ginkgo.BeforeAll(func() {
			//Scale down
			testNdb.Spec.Mysqld = nil
			ndbtest.KubectlApplyNdbObjNoWait(testNdb)
			// validate the status updates made by the operator during the sync
			ndbutils.ValidateNdbClusterStatusUpdatesDuringSync(
				ctx, tc.NdbClientset(), ns, ndbName)
			mysqlpodlist = nil

			//Scale up
			testNdb.Spec.Mysqld = &v1alpha1.NdbMysqldSpec{
				RootHost:  "1.1.0.1",
				NodeCount: 2,
			}
		})

		ginkgo.It("ensure that root user is not accessible from any of the mysql pod in cluster", func() {
			testNdb.Spec.Mysqld = &v1alpha1.NdbMysqldSpec{
				NodeCount: 3,
			}
			cmd := buildExecCmd(ctx, c, testNdb, ns)

			_, _, err := k8sutils.Exec(c, mysqlpodlist.Items[0].Name, ns, cmd)
			ndbtest.ExpectError(err, "Expecting failure but succeed")

			_, _, err = k8sutils.Exec(c, mysqlpodlist.Items[1].Name, ns, cmd)
			ndbtest.ExpectError(err, "Expecting failure but succeed")
		})
	})
})
