// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"
	"fmt"
	"time"

	mysqlutils "github.com/mysql/ndb-operator/e2e-tests/utils/mysql"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	podutils "github.com/mysql/ndb-operator/e2e-tests/utils/pods"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
	ginkgo "github.com/onsi/ginkgo/v2"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
)

var _ = ndbtest.NewOrderedTestCase("MySQL PVC", func(tc *ndbtest.TestContext) {
	var ctx context.Context
	var ns string
	var c clientset.Interface
	var ndbName string
	var testNdb *v1.NdbCluster

	ginkgo.BeforeEach(func() {
		ginkgo.By("extracting values from TestContext")
		ndbName = "example-ndb-mysqld-pvc"
		ns = tc.Namespace()
		c = tc.K8sClientset()
		ctx = tc.Ctx()
		// dummy NdbCluster object to use helper functions
		testNdb = testutils.NewTestNdb(ns, ndbName, 2)
	})

	// When using PVC for the mysqld, test to make sure the data
	// in the mysqld pods were not lost on deletion
	ginkgo.When("the example-ndb-mysqld-pvc yaml is applied", func() {

		ginkgo.BeforeAll(func() {
			ndbtest.KubectlApplyNdbYaml(c, ns, "e2e-tests/test_yaml", ndbName)
			ginkgo.DeferCleanup(func() {
				ndbtest.KubectlDeleteNdbYaml(c, ns, ndbName, "e2e-tests/test_yaml", ndbName)
			})
		})

		ginkgo.It("should deploy MySQL server with PVC and withstand data loss during pod restarts", func() {
			mysqldSfsetName := testNdb.GetWorkloadName(constants.NdbNodeTypeMySQLD)
			mysqldPodName := fmt.Sprintf("%s-0", mysqldSfsetName)

			ginkgo.By("creating user tables in a MySQL server", func() {
				// Connect() connects to mysqld service and mysqld service forwards the
				// request to any of the pods associated with it. So, if we create the table
				// from one pod and try to delete the other pod then the test will fail. But,
				// this will not be an issue with this test case because there is only one
				// mysqld in the configuration, So all the requests from Connect() will go to
				// same pod.
				db := mysqlutils.Connect(c, testNdb, "")
				_, err := db.Exec("create database test")
				ndbtest.ExpectNoError(err, "create database test failed")
				_, err = db.Exec("create table test.t1 (id int, value char(10))")
				ndbtest.ExpectNoError(err, "create table t1 failed")
			})

			ginkgo.By("Deleting the MySQL server pod in which tables are created", func() {
				podRestarted := make(chan bool)
				ctxWithTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				defer cancel()

				// Start a go routine to watch for events in mysql pod
				go func() {
					defer ginkgo.GinkgoRecover()

					watcher, err := c.AppsV1().StatefulSets(ns).Watch(ctxWithTimeout, metav1.ListOptions{
						FieldSelector: fields.Set{
							"metadata.name": mysqldSfsetName,
						}.String(),
					})
					ndbtest.ExpectNoError(err,
						"Creating a watch for statefulset %q failed", mysqldSfsetName)

					// Listen to all events and wait for an event with eventReason to occur
					for {
						select {
						case watchEvent := <-watcher.ResultChan():
							if watchEvent.Type != watch.Modified {
								continue
							}
							sfset := watchEvent.Object.(*appsv1.StatefulSet)
							if sfset.Status.ReadyReplicas == *(sfset.Spec.Replicas) &&
								sfset.Status.UpdatedReplicas == *(sfset.Spec.Replicas) {
								// All pods are ready
								podRestarted <- true
								return
							}
						case <-ctxWithTimeout.Done():
							// wait timeout
							ginkgo.Fail("Timed out waiting for pod to restart")
							close(podRestarted)
						}
					}
				}()

				// Delete the pod
				err := podutils.DeletePodIfExists(ctx, c, ns, mysqldPodName)
				ndbtest.ExpectNoError(err, "failed to delete Pod %s", mysqldPodName)
				// Wait for it to restart and become ready
				<-podRestarted
			})

			ginkgo.By("verifying that user tables persist after the pod restart from the statefulSet controller")
			tableNameList := []string{"t1"}
			mysqlutils.ExpectTablesInDatabase(ctx, c, testNdb, tableNameList, "test")
		})
	})
})
