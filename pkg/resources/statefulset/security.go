// Copyright (c) 2020, 2025, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package statefulset

import (
	"github.com/mysql/ndb-operator/config"
	corev1 "k8s.io/api/core/v1"
)

func podDefaultSecurityContext() *corev1.PodSecurityContext {
	podSecurityContext := &corev1.PodSecurityContext{
		RunAsNonRoot: boolPtr(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
	if !config.UsePlatformAssignedIDs {
		podSecurityContext.RunAsUser = int64Ptr(int64(config.RunAsUser))
		podSecurityContext.RunAsGroup = int64Ptr(int64(config.RunAsGroup))
		podSecurityContext.FSGroup = int64Ptr(int64(config.FSGroup))
	}
	return podSecurityContext
}

func containerDefaultSecurityContext() *corev1.SecurityContext {
	containerSecurityContext := &corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
		Privileged:               boolPtr(false),
		RunAsNonRoot:             boolPtr(true),
		ReadOnlyRootFilesystem:   boolPtr(true),
		AllowPrivilegeEscalation: boolPtr(false),
	}
	if !config.UsePlatformAssignedIDs {
		containerSecurityContext.RunAsUser = int64Ptr(int64(config.RunAsUser))
		containerSecurityContext.RunAsGroup = int64Ptr(int64(config.RunAsGroup))
	}
	return containerSecurityContext
}

func boolPtr(val bool) *bool {
	return &val
}

func int64Ptr(val int64) *int64 {
	return &val
}
