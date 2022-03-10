// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// Copy values from ndbPodSpec into podSpec
func copyPodSpecFromNdbPodSpec(podSpec *corev1.PodSpec, ndbPodSpec *v1alpha1.NdbPodSpec) {
	if ndbPodSpec == nil {
		// Nothing to do
		return
	}

	if podSpec == nil {
		panic("nil podSpec sent to copyPodSpecFromNdbPodSpec")
	}

	// Copy the Resource requests/limits
	if ndbPodSpec.Resources != nil {
		resources := ndbPodSpec.Resources
		// Set the Resources request via the pod's container
		if len(podSpec.Containers) == 0 {
			panic("copyPodSpecFromNdbPodSpec should be called only after the containers are set")
		}
		containerResources := &podSpec.Containers[0].Resources

		if containerResources.Limits == nil {
			containerResources.Limits = make(corev1.ResourceList)
		}

		if containerResources.Requests == nil {
			containerResources.Requests = make(corev1.ResourceList)
		}

		// Copy individual limits and requests separately to ensure that we overwrite
		// the defaults only if the ndbPodSpec has specified a ResourceName explicitly.
		for key, value := range resources.Limits {
			containerResources.Limits[key] = value
		}
		for key, value := range resources.Requests {
			containerResources.Requests[key] = value
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
