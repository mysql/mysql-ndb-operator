// Copyright (c) 2022, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"fmt"
	"strconv"

	"github.com/mysql/ndb-operator/e2e-tests/utils/k8sutils"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	sfset_utils "github.com/mysql/ndb-operator/e2e-tests/utils/statefulset"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	ginkgo "github.com/onsi/ginkgo/v2"

	clientset "k8s.io/client-go/kubernetes"
)

const (
	ndbOperatorHelmChart   = "deploy/charts/ndb-operator"
	ndbOperatorReleaseName = "ndb-operator-rel"
)

type testContext struct {
	ndbOperatorInstalled bool
	ndbCluster           *v1.NdbCluster
	namespace            string
}

func verifyNdbNodesInEachNs(clientset clientset.Interface, namespace string, testNdbCluster *v1.NdbCluster) {
	ginkgo.By("Verifying that all ndb nodes were deployed in each namespace")
	sfset_utils.ExpectHasReplicas(clientset, namespace, testNdbCluster.GetWorkloadName(constants.NdbNodeTypeMgmd), 1)
	sfset_utils.ExpectHasReplicas(clientset, namespace, testNdbCluster.GetWorkloadName(constants.NdbNodeTypeNdbmtd), 2)
	sfset_utils.ExpectHasReplicas(clientset, namespace, testNdbCluster.GetWorkloadName(constants.NdbNodeTypeMySQLD), 2)
}

var _ = ginkgo.Describe("Multiple NDB Clusters released in different Namespaces", func() {
	numOfNamespaces := 2
	tc := ndbtest.NewTestContext()
	var contextArray []testContext
	ginkgo.When("multiple NDB Operators are released multiple times into different namespaces", func() {

		ginkgo.BeforeEach(func() {
			contextArray = make([]testContext, numOfNamespaces)
			for i := 0; i < numOfNamespaces; i++ {
				//create namespace and release the ndb-operator
				namespace := "multi-ns-" + strconv.Itoa(i)
				ginkgo.By(fmt.Sprintf("Creating namespace %q", namespace))
				_, err := k8sutils.CreateNamespace(tc.Ctx(), tc.K8sClientset(), namespace)
				ndbtest.ExpectNoError(err, "create namespace %q returned an error", namespace)
				contextArray[i].namespace = namespace

				//Release operator using Helm
				desc := fmt.Sprintf(
					"Installing NDB Operator, web hook server and related resources in namespace %q", namespace)
				ginkgo.By(desc)
				ndbtest.HelmInstall(namespace, ndbOperatorReleaseName, ndbOperatorHelmChart, true)
				contextArray[i].ndbOperatorInstalled = true
				ndbtest.WaitForNdbOperatorReady(tc.Ctx(), tc.NdbClientset(), namespace)

				testNdbCluster := testutils.NewTestNdb(namespace, fmt.Sprintf("namespace-scope-test-%d", i), 2)
				contextArray[i].ndbCluster = testNdbCluster
				testNdbCluster.Spec.RedundancyLevel = 1

				// Create the NdbCluster Object in K8s and wait for the operator to deploy the MySQL Cluster
				ginkgo.By(
					fmt.Sprintf("Creating NdbCluster '%s/%s'", testNdbCluster.Namespace, testNdbCluster.Name),
					func() {
						ndbtest.KubectlApplyNdbObj(tc.K8sClientset(), testNdbCluster)
					})
			}
		})

		ginkgo.AfterEach(func() {
			for _, newContext := range contextArray {
				namespace := newContext.namespace
				if namespace != "" {
					testNdbCluster := newContext.ndbCluster
					if testNdbCluster != nil {
						// Delete the NdbCluster
						ginkgo.By(
							fmt.Sprintf("Deleting NdbCluster '%s/%s'", testNdbCluster.Namespace, testNdbCluster.Name),
							func() {
								ndbtest.KubectlDeleteNdbObj(tc.K8sClientset(), testNdbCluster)
							})
					}

					if newContext.ndbOperatorInstalled {
						desc := fmt.Sprintf(
							"Uninstalling NDB Operator, web hook server and related resources from namespace %q", namespace)
						ginkgo.By(desc)
						ndbtest.HelmUninstall(namespace, ndbOperatorReleaseName)
					}

					ginkgo.By(fmt.Sprintf("Deleting namespace %q", namespace))
					err := k8sutils.DeleteNamespace(tc.Ctx(), tc.K8sClientset(), namespace)
					ndbtest.ExpectNoError(err, "delete namespace %q returned an error", namespace)
				}
			}

		})

		ginkgo.It("should make all NdbCluster resources available in each namespace", func() {
			for _, newContext := range contextArray {
				testNdbCluster := newContext.ndbCluster
				verifyNdbNodesInEachNs(tc.K8sClientset(), testNdbCluster.Namespace, testNdbCluster)
			}
		})
	})
})
