// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package podutils

import (
	"bufio"
	"bytes"
	"context"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// GetPodNameWithLabel returns the name of the pod that has the given label.
// Note : This function assumes that there is only one pod that has the label specified by the selector.
func GetPodNameWithLabel(ctx context.Context, clientset kubernetes.Interface, namespace string, labelSelector labels.Selector) string {
	podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector.String(),
	})
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "List pods failed with an error")
	gomega.Expect(podList.Items).To(gomega.HaveLen(1), "returned podList has more than 1 pod")
	return podList.Items[0].Name
}

// CollectPodLogs streams all the logs of a pod until it either fails
// or completes and then returns the collected logs in a bytes.Buffer.
// Note : This function will block the caller until the pod stops as
//        the method follows the log stream of the pod.
func CollectPodLogs(ctx context.Context, clientset kubernetes.Interface, namespace, podName string) *bytes.Buffer {
	// Enable follow to stream logs until the pod gets stopped/killed
	podLogOptions := &corev1.PodLogOptions{
		Follow: true,
	}
	// Get the log stream using GetLogs
	podLogStream, err := clientset.CoreV1().Pods(namespace).GetLogs(podName, podLogOptions).Stream(ctx)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "GetLogs for pod %q failed", podName)

	// Use a bufio scanner and a custom splitFunction to read in all the log
	scanner := bufio.NewScanner(podLogStream)
	scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		// Custom split function that returns the entire data without trimming anything
		dataLen := len(data)
		if atEOF && dataLen == 0 {
			return 0, nil, nil
		}
		return dataLen, data, nil
	})

	// Scan and store the log in a byte.Buffer
	var logBuffer bytes.Buffer
	for scanner.Scan() {
		_, err = logBuffer.Write(scanner.Bytes())
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred(),
			"failed to write %q pod's logs to buffer", podName)
	}

	// Scan completed. Check for any errors and return the buffer.
	gomega.Expect(scanner.Err()).To(gomega.Succeed(),
		"scanner returned an error during scan of %q pod's logs", podName)
	return &logBuffer
}

func getConditionStatus(pod *corev1.Pod, conditionType corev1.PodConditionType) corev1.ConditionStatus {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Status
		}
	}
	return corev1.ConditionUnknown
}

// WatchForPodError watches the given pod for any errors or crashes.
// It returns true when the pod stops due to an error or
//            false when the pod completes without an error.
// Note : this method should be called only when the pod is ready.
func WatchForPodError(ctx context.Context, clientset kubernetes.Interface, namespace, podName string) bool {
	// Start watching the pod
	watcher, err := clientset.CoreV1().Pods(namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fields.Set{
			"metadata.name": podName,
		}.String(),
	})
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred(), "Watch request for pod %q failed", podName)
	defer watcher.Stop()

	eventsChan := watcher.ResultChan()
	// Receive the first event and verify that the pod is ready
	event := <-eventsChan
	gomega.Expect(event.Object).NotTo(gomega.BeNil(),
		"watcher sent back an empty event")
	pod := event.Object.(*corev1.Pod)
	if getConditionStatus(pod, corev1.PodReady) != corev1.ConditionTrue {
		panic("pod not ready when WatchForPodError was called")
	}

	// Watch for the events and deduct if the pod is failing
	for {
		event = <-eventsChan
		gomega.Expect(event.Object).NotTo(gomega.BeNil(),
			"watcher sent back an empty event")
		pod = event.Object.(*corev1.Pod)
		if getConditionStatus(pod, corev1.ContainersReady) == corev1.ConditionTrue {
			// All containers are ready => No errors yet.
			// This can happen when the K8s Server marks the Pod
			// for deletion by updating the DeletionTimestamp.
			continue
		}

		// Pod is not ready - check for errors in all containers
		for _, containerStatus := range pod.Status.ContainerStatuses {
			containerState := containerStatus.State
			if containerState.Terminated != nil {
				if containerState.Terminated.ExitCode == 0 {
					// This container has completed without error
					return false
				} else {
					// This container has failed
					return true
				}
			}
		}
	}
}
