// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"strconv"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/resources"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	Separator = "/"
)

// getNamespacedName returns the name of the object
// along with the Namespace of form <namespace>/<name>.
func getNamespacedName(obj v1.Object) string {
	return obj.GetNamespace() + Separator + obj.GetName()
}

// getNdbClusterKey returns a key for the
// given NdbCluster of form <namespace>/<name>.
func getNdbClusterKey(nc *v1alpha1.NdbCluster) string {
	return getNamespacedName(nc)
}

// deploymentComplete considers a deployment to be complete
// once all of its desired replicas are updated and available,
// and no old pods are running.
func deploymentComplete(deployment *appsv1.Deployment) bool {
	return deployment.Status.UpdatedReplicas == *(deployment.Spec.Replicas) &&
		deployment.Status.Replicas == *(deployment.Spec.Replicas) &&
		deployment.Status.AvailableReplicas == *(deployment.Spec.Replicas) &&
		deployment.Status.ObservedGeneration >= deployment.Generation
}

// statefulsetUpdateComplete returns true when all the pods
// controlled by the given statefulSet have been updated to
// the latest version and are ready.
func statefulsetUpdateComplete(statefulset *appsv1.StatefulSet) bool {
	return statefulset.Status.UpdatedReplicas == *(statefulset.Spec.Replicas) &&
		statefulset.Status.ReadyReplicas == *(statefulset.Spec.Replicas) &&
		statefulset.Status.ObservedGeneration >= statefulset.Generation &&
		// CurrentRevision/Replicas not updated if OnDelete update strategy is used.
		// So skip checking statefulset.Status.CurrentReplicas for OnDelete
		// https://github.com/kubernetes/kubernetes/issues/106055
		(statefulset.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType ||
			statefulset.Status.UpdatedReplicas == *(statefulset.Spec.Replicas))
}

// statefulsetReady considers a StatefulSet to be ready
// if all the pods created by the statefulSet are ready.
func statefulsetReady(statefulset *appsv1.StatefulSet) bool {
	return statefulset.Status.ReadyReplicas == *(statefulset.Spec.Replicas) &&
		statefulset.Status.ObservedGeneration == statefulset.Generation
}

// workloadHasConfigGeneration returns true if the expectedConfigGeneration
// has already been applied to the given Deployment/StatefulSet.
func workloadHasConfigGeneration(obj v1.Object, expectedConfigGeneration int64) bool {
	// Get the last applied Config Generation
	annotations := obj.GetAnnotations()
	existingConfigGeneration, _ := strconv.ParseInt(annotations[resources.LastAppliedConfigGeneration], 10, 64)
	return existingConfigGeneration == expectedConfigGeneration
}
