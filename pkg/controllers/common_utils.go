// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"strconv"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/resources/statefulset"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	Separator = "/"
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
func getNdbClusterKey(nc *v1alpha1.NdbCluster) string {
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
