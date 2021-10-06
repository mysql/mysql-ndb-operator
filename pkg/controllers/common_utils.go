// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	Separator = "/"
)

// getNamespacedName returns the name of the object
// along with the Namespace of form <namespace>/<name>.
func getNamespacedName(meta v1.Object) string {
	return meta.GetNamespace() + Separator + meta.GetName()
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

// statefulsetReady considers a StatefulSet to be ready once any ongoing
// rolling upgrade is complete and all the updated pods are ready.
func statefulsetReady(statefulset *appsv1.StatefulSet) bool {
	// CurrentReplicas is set to UpdatedReplicas and they both are equal
	// to statefulset.Spec.Replicas once rolling update is complete.
	return statefulset.Status.UpdatedReplicas == *(statefulset.Spec.Replicas) &&
		statefulset.Status.CurrentReplicas == *(statefulset.Spec.Replicas) &&
		statefulset.Status.ReadyReplicas == *(statefulset.Spec.Replicas) &&
		statefulset.Status.ObservedGeneration >= statefulset.Generation
}
