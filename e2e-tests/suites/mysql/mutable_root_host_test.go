// Copyright (c) 2022, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"
	"fmt"

	"github.com/mysql/ndb-operator/e2e-tests/utils/k8sutils"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbutils"
	secretutils "github.com/mysql/ndb-operator/e2e-tests/utils/secret"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
	ginkgo "github.com/onsi/ginkgo/v2"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = ndbtest.NewOrderedTestCase("Mutable root host", func(tc *ndbtest.TestContext) {
	var ns string
	var c clientset.Interface
	var testNdb *v1.NdbCluster
	var ctx context.Context
	var ndbName string

	var mysqlPod0Name, mysqlPod1Name, mysqlPod0HostName string

	var mysqlCmd []string

	ginkgo.BeforeAll(func() {
		ginkgo.By("extracting values from TestContext")
		ns = tc.Namespace()
		c = tc.K8sClientset()
		ctx = tc.Ctx()

		// Create the NdbCluster object to be used by the testcases
		ndbName = "mutable-roothost-test"
		testNdb = testutils.NewTestNdb(ns, ndbName, 2)

		// create the secret in K8s
		mysqlRootSecretName := ndbName + "-root-secret"
		ginkgo.By("creating MySQL root account secret")
		secretutils.CreateSecret(ctx, c, mysqlRootSecretName, ns)
		testNdb.Spec.MysqlNode.RootPasswordSecretName = mysqlRootSecretName

		mysqlPod0Name = testNdb.GetWorkloadName(constants.NdbNodeTypeMySQLD) + "-0"
		mysqlPod1Name = testNdb.GetWorkloadName(constants.NdbNodeTypeMySQLD) + "-1"

		mysqlPod0HostName = fmt.Sprintf("%s-0.%s.%s",
			testNdb.GetWorkloadName(constants.NdbNodeTypeMySQLD),
			testNdb.GetServiceName(constants.NdbNodeTypeMySQLD), testNdb.Namespace)

		mysqlCmd = []string{
			"sh",
			"-c",
			"mysql --user=root --password=" + secretutils.TestRootPassword +
				" --protocol=tcp --host=" + mysqlPod0HostName +
				" --database=mysql --execute='select user, host from user;'",
		}

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
			ctx, tc.NdbClientset(), ns, ndbName, false)
	})

	// Due to Bug #33904288: host change from % -> any address is not getting distributed
	// So, testing from Unknown ip -> validip -> %

	//CASE 1: RootHost = Unknown ip
	ginkgo.When("Root host in MySQL Server is assigned to ip outside cluster", func() {

		ginkgo.BeforeAll(func() {
			testNdb.Spec.MysqlNode.RootHost = "1.1.0.1"
		})

		ginkgo.It("ensure that root user is not accessible from any of the mysql pod in cluster", func() {
			_, _, err := k8sutils.Exec(c, mysqlPod0Name, ns, mysqlCmd)
			ndbtest.ExpectError(err, "Expecting failure but succeed")

			_, _, err = k8sutils.Exec(c, mysqlPod1Name, ns, mysqlCmd)
			ndbtest.ExpectError(err, "Expecting failure but succeed")
		})
	})

	//CASE 2: RootHost = valid ip
	ginkgo.When("Root host in MySQL Server is assigned to ip of one of the mysql pod in cluster", func() {

		ginkgo.BeforeAll(func() {
			testNdb.Spec.MysqlNode.RootHost = mysqlPod0HostName + ".%"
		})

		ginkgo.It("ensure that root user is accessible from only one pod", func() {
			_, _, err := k8sutils.Exec(c, mysqlPod0Name, ns, mysqlCmd)
			ndbtest.ExpectNoError(err, "Failure executing command %s", mysqlCmd)

			_, _, err = k8sutils.Exec(c, mysqlPod1Name, ns, mysqlCmd)
			ndbtest.ExpectError(err, "Expecting failure but succeed")
		})
	})

	//CASE 3: RootHost = %
	ginkgo.When("Root host in MySQL Server is set to %", func() {

		ginkgo.BeforeAll(func() {
			testNdb.Spec.MysqlNode.RootHost = "%"
		})

		ginkgo.It("ensure that root user is accessible from all pod", func() {
			_, _, err := k8sutils.Exec(c, mysqlPod0Name, ns, mysqlCmd)
			ndbtest.ExpectNoError(err, "Failure executing command %s", mysqlCmd)

			_, _, err = k8sutils.Exec(c, mysqlPod1Name, ns, mysqlCmd)
			ndbtest.ExpectNoError(err, "Failure executing command %s", mysqlCmd)
		})

	})
})
