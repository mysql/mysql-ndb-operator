// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package statefulset

import (
	"strconv"
	"strings"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/ndbconfig"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// NdbStatefulSetInterface defines the methods to be implemented by the StatefulSets of the MySQL Cluster nodes
type NdbStatefulSetInterface interface {
	GetTypeName() constants.NdbNodeType
	GetName(nc *v1alpha1.NdbCluster) string
	NewStatefulSet(cs *ndbconfig.ConfigSummary, cluster *v1alpha1.NdbCluster) *appsv1.StatefulSet
}

type baseStatefulSet struct {
	nodeType constants.NdbNodeType
}

// getContainerName returns the container name
func (bss *baseStatefulSet) getContainerName(initContainer bool) string {
	if initContainer {
		return bss.GetTypeName() + "-init-container"
	} else {
		return bss.GetTypeName() + "-container"
	}
}

// getStatefulSetLabels returns the labels of the StatefulSet
func (bss *baseStatefulSet) getStatefulSetLabels(nc *v1alpha1.NdbCluster) map[string]string {
	return nc.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: bss.nodeType + "-statefulset",
	})
}

// getPodLabels generates the labels of the pods controlled by the StatefulSets
func (bss *baseStatefulSet) getPodLabels(nc *v1alpha1.NdbCluster) map[string]string {
	return nc.GetCompleteLabels(map[string]string{
		constants.ClusterNodeTypeLabel: bss.nodeType,
	})
}

// getEmptyDirVolumeName returns the name of the empty directory volume
func (bss *baseStatefulSet) getEmptyDirVolumeName() string {
	return bss.nodeType + "-empty-dir-volume"
}

// getEmptyDirPodVolumes returns an empty directory pod volume
func (bss *baseStatefulSet) getEmptyDirPodVolume() *corev1.Volume {
	return &corev1.Volume{
		Name: bss.getEmptyDirVolumeName(),
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// getHelperScriptVolumeMount returns the VolumeMount for the helper scripts
func (bss *baseStatefulSet) getHelperScriptVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		// Volume mount for helper scripts
		Name:      helperScriptsVolName,
		MountPath: helperScriptsMountPath,
	}
}

// createContainer creates a new container for the StatefulSet with default values
func (bss *baseStatefulSet) createContainer(
	nc *v1alpha1.NdbCluster, containerName string, commandAndArgs []string,
	volumeMounts []corev1.VolumeMount, portNumbers ...int32) corev1.Container {

	// Expose the ports passed via portNumbers
	var ports []corev1.ContainerPort
	for _, portNumber := range portNumbers {
		ports = append(ports, corev1.ContainerPort{
			ContainerPort: portNumber,
		})
	}

	klog.Infof("Creating container %q from image %s", containerName, nc.Spec.Image)
	return corev1.Container{
		Name: containerName,
		// Use the image provided in spec
		Image:           nc.Spec.Image,
		ImagePullPolicy: nc.Spec.ImagePullPolicy,
		Ports:           ports,
		// Export the Pod IP, Namespace and connectstring to Pod env
		Env: []corev1.EnvVar{
			{
				Name: "NDB_POD_IP",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "status.podIP",
					},
				},
			},
			{
				Name: "NDB_POD_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
			{
				Name:  "NDB_CONNECTSTRING",
				Value: nc.GetConnectstring(),
			},
		},
		Command:      []string{"/bin/bash", "-ecx", strings.Join(commandAndArgs, " ")},
		VolumeMounts: volumeMounts,
	}
}

// newStatefulSet defines a new StatefulSet that will be
// used by the mgmd and ndbd StatefulSets
func (bss *baseStatefulSet) newStatefulSet(
	nc *v1alpha1.NdbCluster, cs *ndbconfig.ConfigSummary) *appsv1.StatefulSet {

	// Fill in the podSpec with any provided ImagePullSecrets
	var podSpec corev1.PodSpec
	imagePullSecretName := nc.Spec.ImagePullSecretName
	if imagePullSecretName != "" {
		podSpec.ImagePullSecrets = []corev1.LocalObjectReference{
			{
				Name: imagePullSecretName,
			},
		}
	}

	// Labels to be used for the statefulset pods
	podLabels := bss.getPodLabels(nc)

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   bss.GetName(nc),
			Labels: bss.getStatefulSetLabels(nc),
			// Owner reference pointing to the Ndb resource
			OwnerReferences: nc.GetOwnerReferences(),
			Annotations: map[string]string{
				// Add the NdbCluster generation this statefulset is based on to the annotation
				LastAppliedConfigGeneration: strconv.FormatInt(cs.NdbClusterGeneration, 10),
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				// Use the pods labels as the selector
				MatchLabels: podLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
					// Annotate the spec template with the config.ini version.
					// A change in the config will create a new version of the spec template.
					Annotations: map[string]string{
						LastAppliedMySQLClusterConfigVersion: strconv.FormatInt(int64(cs.MySQLClusterConfigVersion), 10),
					},
				},
				Spec: podSpec,
			},
			// The services must exist before the StatefulSet,
			// and is responsible for the network identity of the set.
			ServiceName: nc.GetServiceName(bss.nodeType),
		},
	}
}

// GetName returns the name of the baseStatefulSet
func (bss *baseStatefulSet) GetName(nc *v1alpha1.NdbCluster) string {
	return nc.Name + "-" + bss.nodeType
}

// GetTypeName returns the type name of baseStatefulSet
func (bss *baseStatefulSet) GetTypeName() constants.NdbNodeType {
	return bss.nodeType
}
