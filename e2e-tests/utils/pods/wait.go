// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package pods

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"time"
)

const (
	// constant poll interval and timeout for waits
	pollInterval = 1 * time.Second
	pollTimeout  = 10 * time.Minute
)

func getPod(clientset kubernetes.Interface, namespace, name string) *corev1.Pod {
	pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		panic(fmt.Errorf("failed to retrieve pod \"%s.%s\" : %s", namespace, name, err))
	}
	return pod
}

// HasPodSucceeded returns true of the pod succeeded
func HasPodSucceeded(clientset kubernetes.Interface, namespace, name string) bool {
	pod := getPod(clientset, namespace, name)
	return pod != nil && pod.Status.Phase == corev1.PodSucceeded
}

func isPodStarted(clientset kubernetes.Interface, namespace, name string) bool {
	pod := getPod(clientset, namespace, name)
	return pod.Status.Phase != corev1.PodPending
}

func isPodTerminated(clientset kubernetes.Interface, namespace, name string) bool {
	pod := getPod(clientset, namespace, name)
	podPhase := pod.Status.Phase
	return podPhase == corev1.PodSucceeded || podPhase == corev1.PodFailed
}

// podStatusValidatorFunc validates a pod based on some condition
type podStatusValidatorFunc func(clientset kubernetes.Interface, namespace, name string) (success bool)

// WaitForPodPhase waits for a pod to reach the expected state until a timeout
func WaitForPodPhase(clientset kubernetes.Interface,
	namespace, name string, podStatusValidator podStatusValidatorFunc, desc string) (err error) {
	// Wait for the pod phase until the timeout
	for start := time.Now(); time.Since(start) < pollTimeout; time.Sleep(pollInterval) {
		if podStatusValidator(clientset, namespace, name) {
			return nil
		}
	}
	return fmt.Errorf("%s timedout", desc)
}

func WaitForPodToStart(
	clientset kubernetes.Interface, namespace, name string) error {
	return WaitForPodPhase(clientset, namespace, name, isPodStarted, "WaitForPodToStart")
}

func WaitForPodToTerminate(
	clientset kubernetes.Interface, namespace, name string) error {
	return WaitForPodPhase(clientset, namespace, name, isPodTerminated, "WaitForPodToTerminate")
}
