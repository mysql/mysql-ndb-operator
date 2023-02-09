// Copyright (c) 2022, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"
	"fmt"

	"github.com/mysql/ndb-operator/e2e-tests/utils/k8sutils"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	podutils "github.com/mysql/ndb-operator/e2e-tests/utils/pods"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

const (
	testLabelKey = "ndb-worker"
)

func getNodeLabel(id int) string {
	return fmt.Sprintf("wn-%d", id)
}

var _ = ndbtest.NewOrderedTestCase("Node Selectors and Pod affinity", func(tc *ndbtest.TestContext) {
	var ctx context.Context
	var clientset kubernetes.Interface
	var namespace string
	var testNdb *v1.NdbCluster
	var nodeLabels map[string]string

	ginkgo.BeforeAll(func() {
		namespace = tc.Namespace()
		clientset = tc.K8sClientset()
		ctx = tc.Ctx()

		// Add labels to K8s worker nodes
		ginkgo.By("Adding labels to the K8s Worker Nodes")
		nodes := k8sutils.GetWorkerNodes(ctx, clientset)

		if len(nodes) < 3 {
			ginkgo.Skip("Test requires at least 3 worker nodes to run")
		}

		nodeLabels = make(map[string]string)
		for i, node := range nodes {
			labels := node.GetLabels()
			labelValue := getNodeLabel(i)
			labels[testLabelKey] = labelValue
			node.SetLabels(labels)
			k8sutils.UpdateWorkerNode(ctx, clientset, &node)
			// Cache the label => worker name
			nodeLabels[labelValue] = node.GetName()
		}

		testNdb = testutils.NewTestNdb(namespace, "node-spec-test", 2)
		// Add Tolerations to all node types to enable scheduling them
		// on KinD master which has a NoSchedule taint.
		tolerateTaintOnKinDMaster := corev1.Toleration{
			Key:      "node-role.kubernetes.io/master",
			Operator: corev1.TolerationOpEqual,
			Effect:   corev1.TaintEffectNoSchedule,
		}
		testNdb.Spec.ManagementNode.NdbPodSpec = &v1.NdbClusterPodSpec{
			Tolerations: []corev1.Toleration{
				tolerateTaintOnKinDMaster,
			},
		}
		testNdb.Spec.DataNode.NdbPodSpec = &v1.NdbClusterPodSpec{
			Tolerations: []corev1.Toleration{
				tolerateTaintOnKinDMaster,
			},
		}
		testNdb.Spec.MysqlNode.NdbPodSpec = &v1.NdbClusterPodSpec{
			Tolerations: []corev1.Toleration{
				tolerateTaintOnKinDMaster,
			},
		}
		ginkgo.DeferCleanup(func() {
			ginkgo.By("Delete the NdbCluster resource")
			ndbtest.KubectlDeleteNdbObj(clientset, testNdb)
		})
	})

	ginkgo.AfterAll(func() {
		// Remove labels from K8s worker nodes
		ginkgo.By("Removing labels from the K8s Worker Nodes")
		nodes := k8sutils.GetWorkerNodes(ctx, clientset)
		for _, node := range nodes {
			labels := node.GetLabels()
			delete(labels, testLabelKey)
			node.SetLabels(labels)
			k8sutils.UpdateWorkerNode(ctx, clientset, &node)
		}
	})

	ginkgo.When("NodeSelector is used for MySQL Cluster nodes", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.ManagementNode.NdbPodSpec.NodeSelector = map[string]string{
				testLabelKey: getNodeLabel(0),
			}
			testNdb.Spec.DataNode.NdbPodSpec.NodeSelector = map[string]string{
				testLabelKey: getNodeLabel(1),
			}
			testNdb.Spec.MysqlNode.NdbPodSpec.NodeSelector = map[string]string{
				testLabelKey: getNodeLabel(2),
			}
			ndbtest.KubectlApplyNdbObj(clientset, testNdb)
		})

		ginkgo.It("should schedule them in the nodes with specified labels", func() {
			pods := podutils.GetPodsWithLabel(ctx, clientset, namespace, labels.Set(testNdb.GetLabels()).AsSelector())
			for _, pod := range pods {
				nodeType := pod.GetLabels()[constants.ClusterNodeTypeLabel]
				var expectedWorkerNode string
				switch nodeType {
				case constants.NdbNodeTypeMgmd:
					expectedWorkerNode = nodeLabels[getNodeLabel(0)]
				case constants.NdbNodeTypeNdbmtd:
					expectedWorkerNode = nodeLabels[getNodeLabel(1)]
				case constants.NdbNodeTypeMySQLD:
					expectedWorkerNode = nodeLabels[getNodeLabel(2)]
				default:
					panic("unrecognised node type" + nodeType)
				}
				gomega.Expect(pod.Spec.NodeName).Should(gomega.Equal(expectedWorkerNode))
			}
		})
	})

	ginkgo.When("NodeSelector is removed", func() {
		ginkgo.BeforeAll(func() {
			testNdb.Spec.ManagementNode.NdbPodSpec.NodeSelector = nil
			testNdb.Spec.DataNode.NdbPodSpec.NodeSelector = nil
			testNdb.Spec.MysqlNode.NdbPodSpec.NodeSelector = nil
			ndbtest.KubectlApplyNdbObj(clientset, testNdb)
		})

		ginkgo.It("should successfully re-schedule them across all available nodes "+
			"without violating default anti-affinity rules during re-scheduling", func() {
			// i.e. verify that no two mysql cluster nodes of the same type are scheduled together
			pods := podutils.GetPodsWithLabel(ctx, clientset, namespace, labels.Set(testNdb.GetLabels()).AsSelector())
			scheduledWorkerNodes := make(map[string][]string)
			for _, pod := range pods {
				if pod.DeletionTimestamp != nil {
					// This pod is being terminated
					continue
				}
				// Collect similar nodeTypes under a single key
				nodeType := pod.GetLabels()[constants.ClusterNodeTypeLabel]
				scheduledWorkerNodes[nodeType] = append(scheduledWorkerNodes[nodeType], pod.Spec.NodeName)
			}

			// Verify the scheduled nodes are different per nodeType
			for _, workerNodes := range scheduledWorkerNodes {
				gomega.Expect(workerNodes).To(gomega.HaveLen(2))
				gomega.Expect(workerNodes[0]).NotTo(gomega.Equal(workerNodes[1]))
			}
		})
	})
})
