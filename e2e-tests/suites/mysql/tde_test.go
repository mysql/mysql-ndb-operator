// Copyright (c) 2022, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"
	"strings"

	mysqlutils "github.com/mysql/ndb-operator/e2e-tests/utils/mysql"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbutils"
	podutils "github.com/mysql/ndb-operator/e2e-tests/utils/pods"
	secretutils "github.com/mysql/ndb-operator/e2e-tests/utils/secret"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/labels"
	clientset "k8s.io/client-go/kubernetes"
)

func expectDataNodeEncryptionState(
	ctx context.Context, c clientset.Interface, testNdb *v1.NdbCluster, query string, expectedValue bool) {
	db := mysqlutils.Connect(c, testNdb, "")
	row := db.QueryRowContext(ctx, query)
	var value bool
	ndbtest.ExpectNoError(row.Scan(&value),
		"error executing query: %s", query)
	gomega.Expect(value).To(gomega.Equal(expectedValue),
		"unexpected value returned from query: %s expected value: %v", query, expectedValue)
}

func isInitialFlagSetOnDataNodepod(
	ctx context.Context, c clientset.Interface, testNdb *v1.NdbCluster) bool {
	pods := podutils.GetPodsWithLabel(ctx, c, testNdb.Namespace, labels.Set(testNdb.GetLabels()).AsSelector())
	for _, pod := range pods {
		nodeType := pod.GetLabels()[constants.ClusterNodeTypeLabel]
		if nodeType == constants.NdbNodeTypeNdbmtd {
			container := pod.Spec.Containers[0]
			// Check if the Command of the container contains "--initial"
			for _, arg := range container.Command {
				if strings.Contains(arg, "--initial") {
					return true
				}
			}
		}
	}
	return false
}

var _ = ndbtest.NewOrderedTestCase("TDE", func(tc *ndbtest.TestContext) {
	var ns string
	var c clientset.Interface
	var testNdb *v1.NdbCluster
	var ctx context.Context
	var ndbName string

	var query string

	ginkgo.BeforeAll(func() {
		ginkgo.By("extracting values from TestContext")
		ns = tc.Namespace()
		c = tc.K8sClientset()
		ctx = tc.Ctx()

		// Create the NdbCluster object to be used by the testcases
		ndbName = "tde-test"
		testNdb = testutils.NewTestNdb(ns, ndbName, 2)

		query = "SELECT config_value 'EncryptedFileSystem' FROM ndbinfo.config_values cv JOIN ndbinfo.config_params cp ON cv.config_param = cp.param_number WHERE param_name = 'EncryptedFileSystem' LIMIT 1;"

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

	//CASE 1: Default:without TDESecretName
	ginkgo.When("TDESecretName is not set", func() {
		ginkgo.It("ensure that data node file system is not encrypted", func() {
			expectDataNodeEncryptionState(ctx, c, testNdb, query, false)
		})

		ginkgo.It("ensure that data node pods does not have --initial flag in command line argument", func() {
			gomega.Expect(isInitialFlagSetOnDataNodepod(ctx, c, testNdb)).To(
				gomega.BeFalse(), "Datanode has --initial argument set")
		})
	})

	//CASE 2: With TDESecretName
	ginkgo.When("TDESecretName is set to a valid value", func() {

		ginkgo.BeforeAll(func() {
			// create the secret in K8s
			tdeSecretName := ndbName + "-tde-secret-1"
			ginkgo.By("creating TDE secret")
			secretutils.CreateSecretWithPassword(ctx, c, tdeSecretName, ns, "pass123")
			testNdb.Spec.TDESecretName = tdeSecretName
		})

		ginkgo.It("ensure that data node file system is encrypted", func() {
			expectDataNodeEncryptionState(ctx, c, testNdb, query, true)
		})

		ginkgo.It("ensure that data node pods does not have --initial flag in command line argument", func() {
			gomega.Expect(isInitialFlagSetOnDataNodepod(ctx, c, testNdb)).To(
				gomega.BeFalse(), "Datanode has --initial argument set")
		})
	})

	//CASE 3: With different TDE password
	ginkgo.When("TDESecretName is set to different value", func() {

		ginkgo.BeforeAll(func() {
			// create the secret in K8s
			tdeSecretName := ndbName + "-tde-secret-2"
			ginkgo.By("creating second TDE secret")
			secretutils.CreateSecretWithPassword(ctx, c, tdeSecretName, ns, "newpass")
			testNdb.Spec.TDESecretName = tdeSecretName
		})

		ginkgo.It("ensure that data node file system is encrypted", func() {
			expectDataNodeEncryptionState(ctx, c, testNdb, query, true)
		})

		ginkgo.It("ensure that data node pods does not have --initial flag in command line argument", func() {
			gomega.Expect(isInitialFlagSetOnDataNodepod(ctx, c, testNdb)).To(
				gomega.BeFalse(), "Datanode has --initial argument set")
		})
	})

	//CASE 4: unset it back without TDESecretName
	ginkgo.When("TDESecretName is disabled again from the spec", func() {

		ginkgo.BeforeAll(func() {
			testNdb.Spec.TDESecretName = ""
		})

		ginkgo.It("ensure that data node file system is not encrypted", func() {
			expectDataNodeEncryptionState(ctx, c, testNdb, query, false)
		})

		ginkgo.It("ensure that data node pods does not have --initial flag in command line argument", func() {
			gomega.Expect(isInitialFlagSetOnDataNodepod(ctx, c, testNdb)).To(
				gomega.BeFalse(), "Datanode has --initial argument set")
		})
	})
})
