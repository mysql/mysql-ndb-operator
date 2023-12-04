// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/mysql/ndb-operator/e2e-tests/utils/k8sutils"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbutils"
	sfset_utils "github.com/mysql/ndb-operator/e2e-tests/utils/statefulset"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
)

var _ = ndbtest.NewOrderedTestCase("Ndb basic", func(tc *ndbtest.TestContext) {
	var ctx context.Context
	var ns string
	var c clientset.Interface
	var ndbName string
	var testNdb *v1.NdbCluster

	ginkgo.BeforeEach(func() {
		ginkgo.By("extracting values from TestContext")
		ndbName = "example-ndb"
		ns = tc.Namespace()
		c = tc.K8sClientset()
		ctx = tc.Ctx()
		// dummy NdbCluster object to use helper functions
		testNdb = testutils.NewTestNdb(ns, ndbName, 2)
	})

	/*
		Test to make sure we can create and successfully also delete the cluster in the
		example file.

		Its on purpose build on the example
		 - examples should always work and not degrade
		 - the example with 2 nodes of each kind is probably the most common setup
	*/
	ginkgo.When("the example-ndb yaml is applied", func() {

		ginkgo.BeforeAll(func() {
			ndbtest.KubectlApplyNdbYaml(c, ns, "e2e-tests/test_yaml", ndbName)
		})

		ginkgo.AfterAll(func() {
			ndbtest.KubectlDeleteNdbYaml(c, ns, ndbName, "e2e-tests/test_yaml", ndbName)
		})

		ginkgo.It("should deploy MySQL cluster in K8s", func() {

			ginkgo.By("running the correct number of various Ndb nodes")
			sfset_utils.ExpectHasReplicas(c, ns, testNdb.GetWorkloadName(constants.NdbNodeTypeMgmd), 2)
			sfset_utils.ExpectHasReplicas(c, ns, testNdb.GetWorkloadName(constants.NdbNodeTypeNdbmtd), 2)
			sfset_utils.ExpectHasReplicas(c, ns, testNdb.GetWorkloadName(constants.NdbNodeTypeMySQLD), 2)

			ginkgo.By("having the right labels for the pods")
			sfset_utils.ExpectHasLabel(c, ns, testNdb.GetWorkloadName(constants.NdbNodeTypeMgmd), constants.ClusterLabel, "example-ndb")
			sfset_utils.ExpectHasLabel(c, ns, testNdb.GetWorkloadName(constants.NdbNodeTypeNdbmtd), constants.ClusterLabel, "example-ndb")
			sfset_utils.ExpectHasLabel(c, ns, testNdb.GetWorkloadName(constants.NdbNodeTypeMySQLD), constants.ClusterLabel, "example-ndb")

			ginkgo.By("updating the NdbCluster resource status", func() {
				ndbutils.ValidateNdbClusterStatus(ctx, tc.NdbClientset(), ns, ndbName)
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

		// This test checks if the difference between the requested memory and
		// the actual memory for datanode doesn't exceed more than 50%.
		//
		// If the difference is more than 50%, then the formula used to calculate
		// the minimum memory required by the datanode needs to be re-checked.
		ginkgo.It("should set memory requirement to a value not less than 50% of the actual value", func() {

			// Retrieve a data node pod
			ndbmtdPodName := fmt.Sprintf("%s-0", testNdb.GetWorkloadName(constants.NdbNodeTypeNdbmtd))
			pod, err := c.CoreV1().Pods(ns).Get(ctx, ndbmtdPodName, metav1.GetOptions{})
			ndbtest.ExpectNoError(err, "failed to get ndbmtd pod")

			// Extract the memory used by the data node pod.
			// If the pod uses cgroupv1, the memory usage is at
			// /sys/fs/cgroup/memory/memory.usage_in_bytes and for
			// cgroupv2, at it is at /sys/fs/cgroup/memory.current
			cmd := []string{
				"sh",
				"-c",
				"cat /sys/fs/cgroup/memory/memory.usage_in_bytes || cat /sys/fs/cgroup/memory.current",
			}
			stdout, _, err := k8sutils.Exec(c, pod.Name, ns, cmd)
			ndbtest.ExpectNoError(err, "Failure executing command %s", cmd)
			trimString := strings.TrimSuffix(stdout.String(), "\n")
			actualMemory, err := strconv.ParseInt(trimString, 10, 64)
			ndbtest.ExpectNoError(err, "failed to parse actualMemory %s", stdout.String())
			requestedMemory := pod.Spec.Containers[0].Resources.Requests.Memory().Value()
			gomega.Expect(actualMemory/2 < requestedMemory).To(gomega.Equal(true))
		})
	})
})
