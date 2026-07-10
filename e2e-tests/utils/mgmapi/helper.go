// Copyright (c) 2021, 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapiutils

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	podutils "github.com/mysql/ndb-operator/e2e-tests/utils/pods"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/mgmapi"
	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const defaultE2ETestImage = "localhost/e2e-tests"

type nodeStatus struct {
	NodeID          int    `json:"nodeId"`
	NodeType        string `json:"nodeType"`
	IsConnected     bool   `json:"isConnected"`
	NodeGroup       int    `json:"nodeGroup"`
	SoftwareVersion string `json:"softwareVersion"`
}

type valueResult[T uint32 | uint64] struct {
	Value T `json:"value"`
}

func e2eTestImage() string {
	if image := os.Getenv("E2E_TEST_IMAGE"); image != "" {
		return image
	}
	return defaultE2ETestImage
}

func e2eTestImagePullPolicy() corev1.PullPolicy {
	if policy := os.Getenv("E2E_TEST_IMAGE_PULL_POLICY"); policy != "" {
		return corev1.PullPolicy(policy)
	}
	return corev1.PullNever
}

// e2eTestPodNodeName co-locates helpers with the runner when it runs in-cluster,
// removing scheduling variance from network assertions.
func e2eTestPodNodeName(clientset kubernetes.Interface) string {
	podName := os.Getenv("E2E_TEST_POD_NAME")
	podNamespace := os.Getenv("E2E_TEST_POD_NAMESPACE")
	if podName == "" || podNamespace == "" {
		return ""
	}

	pod, err := clientset.CoreV1().Pods(podNamespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	return pod.Spec.NodeName
}

// runMgmAPIClientPod models an ingress peer through its namespace and labels.
func runMgmAPIClientPod(
	clientset kubernetes.Interface,
	ndb *v1.NdbCluster,
	sourceNamespace string,
	podLabels map[string]string,
	args ...string,
) (bool, []byte) {

	podArgs := []string{
		"/usr/local/bin/e2e-mgmapi-client",
		"--connectstring", ndb.GetConnectstring(),
		"--connect-timeout", "3s",
	}
	podArgs = append(podArgs, args...)

	labels := map[string]string{
		"app": "mgmapi-client",
	}
	for key, value := range podLabels {
		labels[key] = value
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: ndb.Name + "-mgmapi-client-",
			Namespace:    sourceNamespace,
			Labels:       labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:            "mgmapi-client",
					Image:           e2eTestImage(),
					ImagePullPolicy: e2eTestImagePullPolicy(),
					Args:            podArgs,
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
			NodeName:      e2eTestPodNodeName(clientset),
		},
	}

	ctx := context.Background()
	createdPod, err := clientset.CoreV1().Pods(sourceNamespace).Create(ctx, pod, metav1.CreateOptions{})
	gomega.Expect(err).Should(gomega.Succeed(), "failed to create mgmapi client pod")
	defer func() {
		err := podutils.DeletePodIfExists(ctx, clientset, createdPod.Namespace, createdPod.Name)
		gomega.Expect(err).Should(gomega.Succeed(), "failed to delete mgmapi client pod")
	}()

	err = podutils.WaitForPodToTerminate(ctx, clientset, createdPod.Namespace, createdPod.Name)
	gomega.Expect(err).Should(gomega.Succeed(), "mgmapi client pod did not terminate")

	logs, err := clientset.CoreV1().Pods(createdPod.Namespace).GetLogs(
		createdPod.Name, &corev1.PodLogOptions{}).DoRaw(ctx)
	gomega.Expect(err).Should(gomega.Succeed(), "failed to collect mgmapi client pod logs")
	return podutils.HasPodSucceeded(ctx, clientset, createdPod.Namespace, createdPod.Name), logs
}

func runMgmAPIClient(
	clientset kubernetes.Interface, ndb *v1.NdbCluster, args ...string) []byte {

	ginkgo.By("running the management client from an explicitly allowed pod")
	succeeded, logs := runMgmAPIClientPod(clientset, ndb, ndb.Namespace, map[string]string{
		constants.ManagementAccessLabel: ndb.Name,
	}, args...)
	gomega.Expect(succeeded).To(gomega.BeTrue(), "mgmapi client pod failed; logs:\n%s", string(logs))

	return logs
}

func nodeTypeFromString(nodeType string) mgmapi.NodeTypeEnum {
	switch nodeType {
	case "NDB":
		return mgmapi.NodeTypeNDB
	case "API":
		return mgmapi.NodeTypeAPI
	case "MGM":
		return mgmapi.NodeTypeMGM
	default:
		panic(fmt.Sprintf("unknown node type %q", nodeType))
	}
}

// GetStatus returns the current status of the MySQL Cluster represented by the ndb object.
func GetStatus(clientset kubernetes.Interface, ndb *v1.NdbCluster) mgmapi.ClusterStatus {
	output := runMgmAPIClient(clientset, ndb, "--operation", "status")

	var nodes []nodeStatus
	gomega.Expect(json.Unmarshal(output, &nodes)).Should(gomega.Succeed(),
		"failed to decode mgmapi status output: %s", string(output))

	status := mgmapi.NewClusterStatus(len(nodes))
	for _, node := range nodes {
		status[node.NodeID] = &mgmapi.NodeStatus{
			NodeType:        nodeTypeFromString(node.NodeType),
			NodeId:          node.NodeID,
			IsConnected:     node.IsConnected,
			NodeGroup:       node.NodeGroup,
			SoftwareVersion: node.SoftwareVersion,
		}
	}
	return status
}

// ExpectClusterHealthy verifies that all management and data nodes are connected.
func ExpectClusterHealthy(clientset kubernetes.Interface, ndb *v1.NdbCluster) {
	gomega.Expect(GetStatus(clientset, ndb).IsHealthy()).To(gomega.BeTrue())
}

// ExpectCanConnectToMgmd verifies that the labeled same-namespace helper pod can connect to mgmd.
func ExpectCanConnectToMgmd(clientset kubernetes.Interface, ndb *v1.NdbCluster) {
	_ = GetStatus(clientset, ndb)
}

// ExpectCannotConnectToMgmd verifies that an unlabeled same-namespace pod cannot connect to mgmd.
func ExpectCannotConnectToMgmd(clientset kubernetes.Interface, ndb *v1.NdbCluster) {
	ginkgo.By("checking that an unlabeled pod cannot access mgmd")
	succeeded, logs := runMgmAPIClientPod(
		clientset, ndb, ndb.Namespace, nil, "--operation", "status")
	gomega.Expect(succeeded).To(gomega.BeFalse(), "unlabeled mgmapi client pod connected; logs:\n%s", string(logs))
}

// ExpectCannotConnectToMgmdFromNamespace verifies that even a pod spoofing the
// operator's reserved labels cannot connect from another namespace.
func ExpectCannotConnectToMgmdFromNamespace(
	clientset kubernetes.Interface, ndb *v1.NdbCluster, sourceNamespace string,
) {
	ginkgo.By("checking that a labeled pod in another namespace cannot access mgmd")
	succeeded, logs := runMgmAPIClientPod(clientset, ndb, sourceNamespace, map[string]string{
		constants.ClusterLabel:          ndb.Name,
		constants.ClusterNodeTypeLabel:  constants.NdbNodeTypeMySQLD,
		constants.ManagementAccessLabel: ndb.Name,
	}, "--operation", "status")
	gomega.Expect(succeeded).To(gomega.BeFalse(),
		"cross-namespace mgmapi client pod connected; logs:\n%s", string(logs))
}

// ForEachConnectedNode runs the given function for every connected node of type nodeType.
func ForEachConnectedNode(
	clientset kubernetes.Interface, ndb *v1.NdbCluster, nodeType mgmapi.NodeTypeEnum,
	testFunc func(nodeId int)) {

	for _, node := range GetStatus(clientset, ndb) {
		if node.IsConnected && node.NodeType == nodeType {
			testFunc(node.NodeId)
		}
	}
}

func getUint32Value(clientset kubernetes.Interface, ndb *v1.NdbCluster, args ...string) uint32 {
	output := runMgmAPIClient(clientset, ndb, args...)
	var result valueResult[uint32]
	gomega.Expect(json.Unmarshal(output, &result)).Should(gomega.Succeed(),
		"failed to decode mgmapi uint32 output: %s", string(output))
	return result.Value
}

func getUint64Value(clientset kubernetes.Interface, ndb *v1.NdbCluster, args ...string) uint64 {
	output := runMgmAPIClient(clientset, ndb, args...)
	var result valueResult[uint64]
	gomega.Expect(json.Unmarshal(output, &result)).Should(gomega.Succeed(),
		"failed to decode mgmapi uint64 output: %s", string(output))
	return result.Value
}

// GetConfigVersion returns the config version reported by mgmd or by a specific node.
func GetConfigVersion(clientset kubernetes.Interface, ndb *v1.NdbCluster, nodeID ...int) uint32 {
	args := []string{"--operation", "config-version"}
	if len(nodeID) > 0 {
		args = append(args, "--node-id", strconv.Itoa(nodeID[0]))
	}
	return getUint32Value(clientset, ndb, args...)
}

// GetDataMemory returns the DataMemory value configured for the data node.
func GetDataMemory(clientset kubernetes.Interface, ndb *v1.NdbCluster, nodeID int) uint64 {
	return getUint64Value(
		clientset, ndb, "--operation", "data-memory", "--node-id", strconv.Itoa(nodeID))
}

// GetMgmdArbitrationRank returns the arbitration rank of a management node.
func GetMgmdArbitrationRank(clientset kubernetes.Interface, ndb *v1.NdbCluster) uint32 {
	return getUint32Value(clientset, ndb, "--operation", "mgmd-arbitration-rank")
}

// ExpectConfigVersionInMySQLClusterNodes checks if the MySQL Cluster nodes run with the expected config version.
func ExpectConfigVersionInMySQLClusterNodes(
	clientset kubernetes.Interface, testNdb *v1.NdbCluster, expectedConfigVersion uint32) {

	ginkgo.By(
		fmt.Sprintf("expecting config version of MySQL Cluster nodes to be %d", expectedConfigVersion))

	gomega.Expect(GetConfigVersion(clientset, testNdb)).To(gomega.Equal(expectedConfigVersion))

	ForEachConnectedNode(clientset, testNdb, mgmapi.NodeTypeNDB, func(nodeId int) {
		gomega.Expect(GetConfigVersion(clientset, testNdb, nodeId)).To(gomega.Equal(expectedConfigVersion))
	})
}
