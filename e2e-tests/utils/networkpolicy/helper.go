// Copyright (c) 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package networkpolicyutils

import (
	"context"
	"fmt"
	"os"
	"time"

	podutils "github.com/mysql/ndb-operator/e2e-tests/utils/pods"
	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultE2ETestImage       = "localhost/e2e-tests"
	probeLabelKey             = "mysql.oracle.com/e2e-network-policy-probe"
	probeServerLabel          = "server"
	probeClientLabel          = "client"
	probePort           int32 = 8080
)

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

// e2eTestPodNodeName keeps both probe endpoints on the runner's node when available.
func e2eTestPodNodeName(ctx context.Context, clientset kubernetes.Interface) string {
	podName := os.Getenv("E2E_TEST_POD_NAME")
	podNamespace := os.Getenv("E2E_TEST_POD_NAMESPACE")
	if podName == "" || podNamespace == "" {
		return ""
	}

	pod, err := clientset.CoreV1().Pods(podNamespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	return pod.Spec.NodeName
}

// runProbeClient returns whether a fresh pod can complete the TCP probe.
func runProbeClient(
	ctx context.Context, clientset kubernetes.Interface, namespace, address, nodeName string) bool {

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "network-policy-probe-client-",
			Namespace:    namespace,
			Labels: map[string]string{
				probeLabelKey: probeClientLabel,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:            "network-policy-probe-client",
					Image:           e2eTestImage(),
					ImagePullPolicy: e2eTestImagePullPolicy(),
					Args: []string{
						"/usr/local/bin/e2e-networkpolicy-probe",
						"--mode", "client",
						"--address", address,
						"--timeout", "3s",
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
			NodeName:      nodeName,
		},
	}

	createdPod, err := clientset.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	gomega.Expect(err).Should(gomega.Succeed(), "failed to create network policy probe client pod")
	defer func() {
		err := podutils.DeletePodIfExists(ctx, clientset, createdPod.Namespace, createdPod.Name)
		gomega.Expect(err).Should(gomega.Succeed(), "failed to delete network policy probe client pod")
	}()

	err = podutils.WaitForPodToTerminate(ctx, clientset, createdPod.Namespace, createdPod.Name)
	gomega.Expect(err).Should(gomega.Succeed(), "network policy probe client pod did not terminate")
	return podutils.HasPodSucceeded(ctx, clientset, createdPod.Namespace, createdPod.Name)
}

func getServerPodIP(ctx context.Context, clientset kubernetes.Interface, namespace, name string) string {
	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	gomega.Expect(err).Should(gomega.Succeed(), "failed to get network policy probe server pod")
	return pod.Status.PodIP
}

func waitForServerPodIP(ctx context.Context, clientset kubernetes.Interface, namespace, name string) string {
	var podIP string
	err := wait.PollUntilContextTimeout(ctx, time.Second, time.Minute, true, func(ctx context.Context) (bool, error) {
		podIP = getServerPodIP(ctx, clientset, namespace, name)
		return podIP != "", nil
	})
	gomega.Expect(err).Should(gomega.Succeed(), "network policy probe server pod did not get an IP")
	return podIP
}

func newDenyIngressPolicy(namespace string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "network-policy-enforcement-probe",
			Namespace: namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					probeLabelKey: probeServerLabel,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress:     []networkingv1.NetworkPolicyIngressRule{},
		},
	}
}

// IsNetworkPolicyEnforced returns true when a deny-ingress NetworkPolicy blocks a probe connection.
func IsNetworkPolicyEnforced(ctx context.Context, clientset kubernetes.Interface, namespace string) bool {
	ginkgo.By("checking whether the CNI enforces NetworkPolicy")

	nodeName := e2eTestPodNodeName(ctx, clientset)
	serverPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "network-policy-probe-server-",
			Namespace:    namespace,
			Labels: map[string]string{
				probeLabelKey: probeServerLabel,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:            "network-policy-probe-server",
					Image:           e2eTestImage(),
					ImagePullPolicy: e2eTestImagePullPolicy(),
					Args: []string{
						"/usr/local/bin/e2e-networkpolicy-probe",
						"--mode", "server",
						"--listen", fmt.Sprintf(":%d", probePort),
					},
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: probePort,
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
			NodeName:      nodeName,
		},
	}

	createdServerPod, err := clientset.CoreV1().Pods(namespace).Create(ctx, serverPod, metav1.CreateOptions{})
	gomega.Expect(err).Should(gomega.Succeed(), "failed to create network policy probe server pod")
	defer func() {
		err := podutils.DeletePodIfExists(ctx, clientset, createdServerPod.Namespace, createdServerPod.Name)
		gomega.Expect(err).Should(gomega.Succeed(), "failed to delete network policy probe server pod")
	}()

	serverAddress := fmt.Sprintf("%s:%d",
		waitForServerPodIP(ctx, clientset, createdServerPod.Namespace, createdServerPod.Name), probePort)

	// Prove baseline reachability so a later failure is attributable to the policy.
	err = wait.PollUntilContextTimeout(ctx, time.Second, time.Minute, true, func(ctx context.Context) (bool, error) {
		return runProbeClient(ctx, clientset, namespace, serverAddress, nodeName), nil
	})
	gomega.Expect(err).Should(gomega.Succeed(), "network policy probe server was not reachable before policy creation")

	networkPolicy := newDenyIngressPolicy(namespace)
	_, err = clientset.NetworkingV1().NetworkPolicies(namespace).Create(ctx, networkPolicy, metav1.CreateOptions{})
	gomega.Expect(err).Should(gomega.Succeed(), "failed to create network policy enforcement probe")
	defer func() {
		err := clientset.NetworkingV1().NetworkPolicies(namespace).Delete(ctx, networkPolicy.Name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			gomega.Expect(err).Should(gomega.Succeed(), "failed to delete network policy enforcement probe")
		}
	}()

	// Network policy programming is asynchronous. Require a denied connection
	// within a bounded window instead of relying on a fixed propagation delay.
	err = wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true,
		func(ctx context.Context) (bool, error) {
			return !runProbeClient(ctx, clientset, namespace, serverAddress, nodeName), nil
		})
	return err == nil
}
