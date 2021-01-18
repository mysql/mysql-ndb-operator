// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"

	crd_utils "github.com/mysql/ndb-operator/e2e-tests/utils/crd"
	deployment_utils "github.com/mysql/ndb-operator/e2e-tests/utils/deployment"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbutils"
	sfset_utils "github.com/mysql/ndb-operator/e2e-tests/utils/statefulset"
	"github.com/mysql/ndb-operator/pkg/constants"
	ndbclientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
)

var _ = ndbtest.NewTestCase("Ndb basic", func(tc *ndbtest.TestContext) {
	var ns string
	var c clientset.Interface
	var ndbName string

	ginkgo.BeforeEach(func() {
		ginkgo.By("extracting values from TestContext")
		ndbName = "example-ndb"
		ns = tc.Namespace()
		c = tc.K8sClientset()
	})

	/*
		Test to make sure we can create and successfully also delete the cluster in the
		example file.

		Its on purpose build on the example
		 - examples should always work and not degrade
		 - the example with 2 nodes of each kind is probably the most common setup
	*/
	ginkgo.When("the example-ndb yaml is applied", func() {

		ginkgo.BeforeEach(func() {
			ndbtest.KubectlApplyNdbYaml(c, ns, "docs/examples", ndbName)
		})

		ginkgo.AfterEach(func() {
			ndbtest.KubectlDeleteNdbYaml(c, ns, ndbName, "docs/examples", ndbName)
		})

		ginkgo.It("should deploy MySQL cluster in K8s", func() {

			ginkgo.By("running the correct number of various Ndb nodes")
			sfset_utils.ExpectHasReplicas(c, ns, "example-ndb-mgmd", 2)
			sfset_utils.ExpectHasReplicas(c, ns, "example-ndb-ndbd", 2)
			deployment_utils.ExpectHasReplicas(c, ns, "example-ndb-mysqld", 2)

			ginkgo.By("having the right labels for the pods")
			sfset_utils.ExpectHasLabel(c, ns, "example-ndb-ndbd", constants.ClusterLabel, "example-ndb")
			sfset_utils.ExpectHasLabel(c, ns, "example-ndb-mgmd", constants.ClusterLabel, "example-ndb")
			deployment_utils.ExpectHasLabel(c, ns, "example-ndb-mysqld", constants.ClusterLabel, "example-ndb")

			ginkgo.By("updating the NdbCluster resource status", func() {
				ndbutils.ValidateNdbClusterStatus(tc.Ctx(), tc.NdbClientset(), ns, ndbName)
			})

			ginkgo.By("verifying that 'kubectl get ndb' reports the status of ndbcluster resource", func() {
				// Expected Output
				// NAME          REPLICA   MANAGEMENT NODES   DATA NODES   MYSQL SERVERS   AGE   UP-TO-DATE
				// example-ndb   2         Ready:2/2          Ready:2/2    Ready:2/2       81s   True
				response := ndbtest.KubectlGet(ns, "ndb", ndbName)
				gomega.Expect(response).To(
					gomega.MatchRegexp(
						".*\nexample-ndb[ ]+2[ ]+Ready:2/2[ ]+Ready:2/2[ ]+Ready:2/2.*True"))
			})
		})
	})

	// TODO: Move this into a separate testcase and add more validation tests
	ginkgo.When("a Ndb with a wrong config is applied", func() {
		var ndbclient ndbclientset.Interface
		ginkgo.BeforeEach(func() {
			ndbclient = tc.NdbClientset()
		})

		ginkgo.It("should return an error", func() {
			var err error

			ndbobj := crd_utils.NewTestNdbCrd(ns, "test-ndb", 1, 2, 2)
			_, err = ndbclient.MysqlV1alpha1().NdbClusters(ns).Create(context.TODO(), ndbobj, metav1.CreateOptions{})
			ndbtest.ExpectError(err)
			gomega.Expect(err.Error()).Should(gomega.ContainSubstring(
				"Invalid value: 1: spec.nodeCount should be a multiple of the spec.redundancyLevel(=2)"))
		})

	})
})
