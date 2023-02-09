// Copyright (c) 2022, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbutils

import (
	"context"
	"fmt"
	"time"

	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	ndbclientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"

	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
)

// expectValidInfoInStatus verifies if all the status fields have valid values
func expectValidInfoInStatus(nc *v1.NdbCluster, initialSystemRestart bool) string {
	status := &nc.Status
	allNodesReady := nc.Generation == status.ProcessedGeneration
	var receivedReason string

	// Loop the three NodeReady status variables and validate their values
	for _, s := range []*struct {
		field, value string
		totalNodes   int32
	}{
		{
			field:      "status.readyManagementNodes",
			value:      status.ReadyManagementNodes,
			totalNodes: nc.GetManagementNodeCount(),
		},
		{
			field:      "status.readyDataNodes",
			value:      status.ReadyDataNodes,
			totalNodes: nc.Spec.DataNode.NodeCount,
		},
		{
			field:      "status.readyMySQLServers",
			value:      status.ReadyMySQLServers,
			totalNodes: nc.GetMySQLServerNodeCount(),
		},
	} {
		// Choose matcher based on allNodesReady status
		var matcher types.GomegaMatcher
		if allNodesReady {
			// Expect Ready:n/n status
			matcher = gomega.Equal(fmt.Sprintf("Ready:%d/%d", s.totalNodes, s.totalNodes))
		} else {
			// Sync ongoing - expect some nodes to be ready i.e. Ready:[0-9]+/n
			matcher = gomega.MatchRegexp("Ready:[0-9]+/%d", s.totalNodes)
		}
		// Validate the status
		gomega.Expect(s.value).To(matcher,
			fmt.Sprintf("%s has an unexpected value", s.field))
	}

	// Verify the values set to conditions
	for _, condition := range status.Conditions {
		switch condition.Type {
		case v1.NdbClusterUpToDate:
			var expectedReasons []string
			expectedStatus := corev1.ConditionFalse
			expectedReasons = append(expectedReasons, v1.NdbClusterUptoDateReasonSpecUpdateInProgress)
			expectedReasons = append(expectedReasons, v1.NdbClusterUptoDateReasonError)
			if allNodesReady {
				expectedStatus = corev1.ConditionTrue
				expectedReasons = append(expectedReasons, v1.NdbClusterUptoDateReasonSyncSuccess)
			} else if initialSystemRestart {
				expectedReasons = append(expectedReasons, v1.NdbClusterUptoDateReasonISR)
				expectedReasons = append(expectedReasons, v1.NdbClusterUptoDateReasonError)
			}
			gomega.Expect(condition.Status).To(
				gomega.Equal(expectedStatus), "NdbClusterUpToDate condition has an invalid status")
			gomega.Expect(condition.Reason).Should(
				gomega.BeElementOf(expectedReasons),
				"NdbClusterUpToDate condition has an invalid reason")
			gomega.Expect(condition.LastTransitionTime.IsZero()).NotTo(
				gomega.BeTrue(), "NdbClusterUpToDate condition LastTransitionTime is not set")

			receivedReason = condition.Reason
		}
	}

	// Verify generatedRootPasswordSecretName value if the sync is complete
	if allNodesReady {
		if nc.GetMySQLServerNodeCount() != 0 && nc.Spec.MysqlNode.RootPasswordSecretName == "" {
			// Expect generated secret name if MySQL Server exist in spec
			// and the spec doesn't have any rootPasswordSecretName.
			gomega.Expect(status.GeneratedRootPasswordSecretName).To(
				gomega.Equal(nc.Name+"-mysqld-root-password"),
				"status.generatedRootPasswordSecretName has unexpected value")
		} else {
			// Expect it to be empty otherwise.
			gomega.Expect(status.GeneratedRootPasswordSecretName).To(
				gomega.BeEmpty(), "status.generatedRootPasswordSecretName has unexpected value")
		}
	}
	return receivedReason
}

const (
	watchTimeout = 15 * time.Minute
)

// ValidateNdbClusterStatusUpdatesDuringSync validates all updates done
// to a NdbCluster status when a sync is ongoing. If waitForSyncError is
// set, the function waits for the error to occur and returns when the error
// is detected. If waitForSyncError is not set, the function returns when the
// NdbCluster spec is in sync with the MySQL Cluster
func ValidateNdbClusterStatusUpdatesDuringSync(
	ctx context.Context, ndbClient ndbclientset.Interface, ns, name string, waitForSyncError bool) {
	var previousReason string
	var errorCount int
	// create a context with timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, watchTimeout)
	defer cancel()

	// Start watching for changes to the NdbCluster resource
	watcher, err := ndbClient.MysqlV1().NdbClusters(ns).Watch(
		ctxWithTimeout, metav1.ListOptions{
			FieldSelector: fields.OneTermEqualSelector("metadata.name", name).String(),
		})
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	// Skip validating the first event as that just establishes the initial state
	watchEvent := <-watcher.ResultChan()
	nc := watchEvent.Object.(*v1.NdbCluster)
	// MySQL Cluster nodes are going through ISR if this is the first generation being processed
	initialSystemRestart := nc.Status.ProcessedGeneration == 0

	// Loop and verify all other status updates until the sync completes
	for nc.Generation != nc.Status.ProcessedGeneration {
		select {
		case <-ctxWithTimeout.Done():
			// Timed out waiting for status updates
			ginkgo.Fail("Timed out waiting for NdbCluster status updates")
		case watchEvent = <-watcher.ResultChan():
			if watchEvent.Type == watch.Modified {
				// Validate the Node ready status
				nc = watchEvent.Object.(*v1.NdbCluster)
				receivedReason := expectValidInfoInStatus(nc, initialSystemRestart)
				if waitForSyncError {
					// Sometimes we can get error during init phase due to DNS issues
					// So, we declare error only on receiving sync error for 3 consecutive
					// ndb updates
					if previousReason != v1.NdbClusterUptoDateReasonError {
						errorCount = 0
					}
					if receivedReason == v1.NdbClusterUptoDateReasonError {
						errorCount++
					}
					if errorCount >= 3 {
						return
					}
					previousReason = receivedReason
				}
			}
		}
	}
}

// ValidateNdbClusterStatus validates the final NdbCluster status when the sync is complete
func ValidateNdbClusterStatus(
	ctx context.Context, ndbClient ndbclientset.Interface, ns, name string) {

	// Get the NdbCluster resource for K8s
	nc, err := ndbClient.MysqlV1().NdbClusters(ns).Get(ctx, name, metav1.GetOptions{})
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	// Fail if a sync is ongoing
	gomega.Expect(nc.Status.ProcessedGeneration).To(gomega.Equal(nc.Generation),
		"ValidateNdbClusterStatus should be called only after the NdbCluster spec has synced")
	expectValidInfoInStatus(nc, false)
}
