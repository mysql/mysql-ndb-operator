// Copyright (c) 2022, 2024, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package statefulset

import (
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	corev1 "k8s.io/api/core/v1"
)

func addResourcesToContainer(container *corev1.Container, resources *corev1.ResourceRequirements) {
	if container.Resources.Limits == nil {
		container.Resources.Limits = make(corev1.ResourceList)
	}
	if container.Resources.Requests == nil {
		container.Resources.Requests = make(corev1.ResourceList)
	}

	// Copy individual limits and requests separately to ensure that we overwrite
	// the defaults only if the ndbPodSpec has specified a ResourceName explicitly.
	for key, value := range resources.Limits {
		container.Resources.Limits[key] = value
	}
	for key, value := range resources.Requests {
		container.Resources.Requests[key] = value
	}
}

// Copy values from ndbPodSpec into podSpec
func CopyPodSpecFromNdbPodSpec(podSpec *corev1.PodSpec, ndbPodSpec *v1.NdbClusterPodSpec) {
	if ndbPodSpec == nil {
		// Nothing to do
		return
	}

	if podSpec == nil {
		panic("nil podSpec sent to CopyPodSpecFromNdbPodSpec")
	}

	// Copy the Resource requests/limits
	if ndbPodSpec.Resources != nil {
		resources := ndbPodSpec.Resources
		// Set the Resources request via the pod's container
		if len(podSpec.Containers) == 0 {
			panic("CopyPodSpecFromNdbPodSpec should be called only after the containers are set")
		}

		// Add resources to main containers
		addResourcesToContainer(&podSpec.Containers[0], resources)

		// Add resources to init containers
		// Kubernetes identifies the highest value for each resource among all init containers, then compares
		// it with the sum of resource values across the containers and selects the highest value
		// to determine node scheduling for this pod. So, setting the resource field of all init
		// containers same as the main container does not impact the node scheduling process. But, this hack
		// is essential because OpenShift raises warnings when a container lacks a resource field.
		for i := range podSpec.InitContainers {
			addResourcesToContainer(&podSpec.InitContainers[i], resources)
		}
	}

	// Copy the NodeSelector completely as the operator won't be setting any default values on it
	if len(ndbPodSpec.NodeSelector) != 0 {
		podSpec.NodeSelector = make(map[string]string)
		for key, value := range ndbPodSpec.NodeSelector {
			podSpec.NodeSelector[key] = value
		}
	}

	// Copy Affinities one by one and preserve any default values
	// if that particular Affinity has not been set in ndbPodSpec
	if ndbPodSpec.Affinity != nil {
		if podSpec.Affinity == nil {
			podSpec.Affinity = new(corev1.Affinity)
		}

		// Handle NodeAffinity
		if ndbPodSpec.Affinity.NodeAffinity != nil {
			podSpec.Affinity.NodeAffinity = ndbPodSpec.Affinity.NodeAffinity.DeepCopy()
		}

		// Handle PodAffinity
		if ndbPodSpec.Affinity.PodAffinity != nil {
			podSpec.Affinity.PodAffinity = ndbPodSpec.Affinity.PodAffinity.DeepCopy()
		}

		// Handle PodAntiAffinity
		if ndbPodSpec.Affinity.PodAntiAffinity != nil {
			podSpec.Affinity.PodAntiAffinity = ndbPodSpec.Affinity.PodAntiAffinity.DeepCopy()
		}
	}

	// Copy the scheduler name as it is
	podSpec.SchedulerName = ndbPodSpec.SchedulerName

	// Copy all the Tolerations
	podSpec.Tolerations = append(podSpec.Tolerations, ndbPodSpec.Tolerations...)
}
