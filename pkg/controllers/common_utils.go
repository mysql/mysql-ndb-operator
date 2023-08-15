// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"fmt"
	"strconv"
	"strings"

	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/resources/statefulset"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	PodInitializing   = "PodInitializing"
	ContainerCreating = "ContainerCreating"
	Separator         = "/"
)

// getNamespacedName returns the name of the object
// along with the Namespace of form <namespace>/<name>.
func getNamespacedName(obj metav1.Object) string {
	return obj.GetNamespace() + Separator + obj.GetName()
}

// getNamespacedName2 combines and returns the name and
// namespace in the form <namespace>/<name>.
func getNamespacedName2(namespace, name string) string {
	return namespace + Separator + name
}

// getNdbClusterKey returns a key for the
// given NdbCluster of form <namespace>/<name>.
func getNdbClusterKey(nc *v1.NdbCluster) string {
	return getNamespacedName(nc)
}

// statefulsetUpdateComplete returns true when all the pods
// controlled by the given statefulSet have been updated to
// the latest version and are ready.
func statefulsetUpdateComplete(statefulset *appsv1.StatefulSet) bool {
	return statefulset.Status.Replicas == *(statefulset.Spec.Replicas) &&
		statefulset.Status.ReadyReplicas == *(statefulset.Spec.Replicas) &&
		statefulset.Status.UpdatedReplicas == *(statefulset.Spec.Replicas) &&
		statefulset.Status.ObservedGeneration >= statefulset.Generation &&
		// CurrentRevision/Replicas not updated if OnDelete update strategy is used.
		// So skip checking statefulset.Status.CurrentReplicas for OnDelete
		// https://github.com/kubernetes/kubernetes/issues/106055
		(statefulset.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType ||
			statefulset.Status.CurrentReplicas == *(statefulset.Spec.Replicas))
}

// isInitialFlagSet() returns true when the StatefulSet container
// specification includes the --initial flag and returns false in all other cases
func isInitialFlagSet(dataNodeSfSet *appsv1.StatefulSet) bool {

	// check if the StatefulSet is valid
	if dataNodeSfSet == nil {
		return false
	}

	container := dataNodeSfSet.Spec.Template.Spec.Containers[0]
	// Check if the Command of the container contains "--initial"
	for _, arg := range container.Command {
		if strings.Contains(arg, "--initial") {
			return true
		}
	}

	return false
}

// statefulsetReady considers a StatefulSet to be ready if all the pods
// created by the statefulSet are ready. Note that this doesn't check if
// the pods created by the StatefulSet are running the latest revision of
// pod spec.
func statefulsetReady(statefulset *appsv1.StatefulSet) bool {
	return statefulset.Status.ReadyReplicas == *(statefulset.Spec.Replicas) &&
		statefulset.Status.ObservedGeneration == statefulset.Generation
}

// workloadHasConfigGeneration returns true if the expectedConfigGeneration
// has already been applied to the given Deployment/StatefulSet.
func workloadHasConfigGeneration(obj metav1.Object, expectedConfigGeneration int64) bool {
	// Get the last applied Config Generation
	annotations := obj.GetAnnotations()
	existingConfigGeneration, _ := strconv.ParseInt(annotations[statefulset.LastAppliedConfigGeneration], 10, 64)
	return existingConfigGeneration == expectedConfigGeneration
}

// getPodCondition returns the PodCondition of given type.
func getPodCondition(pod *corev1.Pod, conditionType corev1.PodConditionType) *corev1.PodCondition {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

// checkContainersForError returns any errors in the given container statuses
func checkContainersForError(containerStatuses []corev1.ContainerStatus, podName string) (errs []string) {
	for _, containerStatus := range containerStatuses {
		containerState := containerStatus.State
		if containerStatus.Ready ||
			containerState.Running != nil {
			// Container is either ready or running
			// No errors yet in the current run.
			continue
		}

		if containerState.Waiting != nil {
			if containerState.Waiting.Reason != PodInitializing &&
				containerState.Waiting.Reason != ContainerCreating {
				msg := fmt.Sprintf("pod %q struck in waiting state : %s", podName,
					containerState.Waiting.Reason)
				if containerState.Waiting.Message != "" {
					// Append the message to err
					msg += " : " + containerState.Waiting.Message
				}
				errs = append(errs, msg)
			}

			// Container is waiting, check for failure in LastTerminationState if any
			containerState = containerStatus.LastTerminationState
		}

		// Check if the container was terminated and if terminated, check the exit code :
		// only 0 (graceful shutdown) and 137 (killed by SIGKILL) are allowed.
		if containerState.Terminated != nil &&
			containerState.Terminated.ExitCode != 0 && containerState.Terminated.ExitCode != 137 {
			msg := fmt.Sprintf("pod %q failed : container %q terminated with exit code %d",
				podName, containerStatus.Name, containerState.Terminated.ExitCode)
			if containerState.Terminated.Message != "" {
				// Append the message to err
				msg += " : " + containerState.Terminated.Message
			}
			errs = append(errs, msg)
		}
	}

	return errs
}

// getPodErrors returns all errors currently faced by the pod or the containers running in it.
func getPodErrors(pod *corev1.Pod) (errs []string) {

	podReadyCondition := getPodCondition(pod, corev1.PodReady)
	if podReadyCondition != nil && podReadyCondition.Status == corev1.ConditionTrue {
		// Pod is ready => no error
		return nil
	}

	// Check if the pod has been scheduled
	podScheduledCondition := getPodCondition(pod, corev1.PodScheduled)
	if podScheduledCondition == nil {
		// Pod has been just added
		return nil
	} else if podScheduledCondition.Status != corev1.ConditionTrue {
		// Pod is not scheduled yet.
		// Check if it is unschedulable.
		if podScheduledCondition.Reason == corev1.PodReasonUnschedulable {
			// This pod is unschedulable due to some reason
			return []string{fmt.Sprintf("pod %q is unschedulable : %s",
				getNamespacedName(pod), podScheduledCondition.Message)}
		}

		// Pod has not been scheduled yet, but it is not unschedulable => no error
		return nil
	}

	// Pod has been scheduled but not ready.
	// Some or all of the containers must have been started.
	// Retrieve and return any errors from them
	errs = append(errs, checkContainersForError(pod.Status.InitContainerStatuses, pod.Name)...)
	errs = append(errs, checkContainersForError(pod.Status.ContainerStatuses, pod.Name)...)
	return errs
}
