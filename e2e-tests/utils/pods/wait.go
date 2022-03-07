// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package podutils

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"time"
)

const (
	// constant poll interval and timeout for waits
	pollInterval = 1 * time.Second
	pollTimeout  = 10 * time.Minute
)

func getPod(ctx context.Context, clientset kubernetes.Interface, namespace, name string) *corev1.Pod {
	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		panic(fmt.Errorf("failed to retrieve pod \"%s.%s\" : %s", namespace, name, err))
	}
	return pod
}

// HasPodSucceeded returns true of the pod succeeded
func HasPodSucceeded(ctx context.Context, clientset kubernetes.Interface, namespace, name string) bool {
	pod := getPod(ctx, clientset, namespace, name)
	return pod != nil && pod.Status.Phase == corev1.PodSucceeded
}

func isPodStarted(ctx context.Context, clientset kubernetes.Interface, namespace, name string) bool {
	pod := getPod(ctx, clientset, namespace, name)
	return pod.Status.Phase != corev1.PodPending
}

func isPodTerminated(ctx context.Context, clientset kubernetes.Interface, namespace, name string) bool {
	pod := getPod(ctx, clientset, namespace, name)
	podPhase := pod.Status.Phase
	return podPhase == corev1.PodSucceeded || podPhase == corev1.PodFailed
}

// podStatusValidatorFunc validates a pod based on some condition
type podStatusValidatorFunc func(ctx context.Context, clientset kubernetes.Interface, namespace, name string) (success bool)

// WaitForPodPhase waits for a pod to reach the expected state until a timeout
func WaitForPodPhase(ctx context.Context, clientset kubernetes.Interface,
	namespace, name string, podStatusValidator podStatusValidatorFunc, desc string) (err error) {
	// Wait for the pod phase until the timeout
	for start := time.Now(); time.Since(start) < pollTimeout; time.Sleep(pollInterval) {
		if podStatusValidator(ctx, clientset, namespace, name) {
			return nil
		}
	}
	return fmt.Errorf("%s timedout", desc)
}

func WaitForPodToStart(
	ctx context.Context, clientset kubernetes.Interface, namespace, name string) error {
	return WaitForPodPhase(ctx, clientset, namespace, name, isPodStarted, "WaitForPodToStart")
}

func WaitForPodToTerminate(
	ctx context.Context, clientset kubernetes.Interface, namespace, name string) error {
	return WaitForPodPhase(ctx, clientset, namespace, name, isPodTerminated, "WaitForPodToTerminate")
}

// DeletePodIfExists deletes a pod if it exists
func DeletePodIfExists(ctx context.Context, clientset kubernetes.Interface, namespace, name string) error {
	podInterface := clientset.CoreV1().Pods(namespace)
	_, err := podInterface.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Pod doesn't exist - nothing to do
			return nil
		} else {
			// Unexpected error
			return err
		}
	}

	// Pod exists - delete it
	if err = podInterface.Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return err
	}

	return nil
}
