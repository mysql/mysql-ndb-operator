// Copyright (c) 2020, 2024, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package statefulset

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/ndbconfig"
	"github.com/mysql/ndb-operator/pkg/resources"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klog "k8s.io/klog/v2"
)

// NdbStatefulSetInterface defines the methods to be implemented by the StatefulSets of the MySQL Cluster nodes
type NdbStatefulSetInterface interface {
	GetTypeName() constants.NdbNodeType
	GetName(nc *v1.NdbCluster) string
	NewGoverningService(nc *v1.NdbCluster) *corev1.Service
	NewStatefulSet(cs *ndbconfig.ConfigSummary, nc *v1.NdbCluster) (*appsv1.StatefulSet, error)
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
func (bss *baseStatefulSet) getStatefulSetLabels(nc *v1.NdbCluster) map[string]string {
	return nc.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: bss.nodeType + "-statefulset",
	})
}

// getPodLabels generates the labels of the pods controlled by the StatefulSets
func (bss *baseStatefulSet) getPodLabels(nc *v1.NdbCluster) map[string]string {
	return nc.GetCompleteLabels(map[string]string{
		constants.ClusterNodeTypeLabel: bss.nodeType,
	})
}

// getDataDirVolumeName returns the data dir volume name
func (bss *baseStatefulSet) getDataDirVolumeName() string {
	return bss.nodeType + "-data-vol"
}

// getEmptyDirPodVolumes returns an empty directory pod volume
func (bss *baseStatefulSet) getEmptyDirPodVolume(name string) *corev1.Volume {
	return &corev1.Volume{
		Name: name,
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
	nc *v1.NdbCluster, containerName string, commandAndArgs []string,
	volumeMounts []corev1.VolumeMount, portNumbers []int32) corev1.Container {

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
				Name: "NDB_CONNECTSTRING",
				Value: fmt.Sprintf("nodeid=%d,%s",
					constants.NdbOperatorDedicatedAPINodeId, nc.GetConnectstring()),
			},
		},
		Command:      []string{"/bin/bash", "-ecx", strings.Join(commandAndArgs, " ")},
		VolumeMounts: volumeMounts,
	}
}

// getWorkDirVolumeMount returns the VolumeMount for the work directory
func (bss *baseStatefulSet) getWorkDirVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      workDirVolName,
		MountPath: workDirVolMount,
	}
}

// getDefaultInitContainers returns the default init containers to be run
func (bss *baseStatefulSet) getDefaultInitContainers(nc *v1.NdbCluster) []corev1.Container {

	// The ndb-pod-initializer is tool is the only default container to be run
	cmdAndArgs := []string{
		"ndb-pod-initializer",
	}

	// Load just the work dir volume mount
	volumeMounts := []corev1.VolumeMount{
		bss.getWorkDirVolumeMount(),
	}

	container := bss.createContainer(nc, "ndb-pod-init-container", cmdAndArgs, volumeMounts, nil)

	// Export connection pool size to env for MySQL type pods
	if bss.GetTypeName() == constants.NdbNodeTypeMySQLD {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "NDB_CONNECTION_POOL_SIZE",
			Value: strconv.Itoa(int(nc.GetMySQLServerConnectionPoolSize())),
		})
	}

	// Append the NDB operator password to the env variable of the ndb-pod-init-container
	container.Env = append(container.Env, corev1.EnvVar{
		Name: "NDB_OPERATOR_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: resources.GetMySQLNDBOperatorPasswordSecretName(nc),
				},
				Key: corev1.BasicAuthPasswordKey,
			},
		},
	})

	// Use the current ndb operator image name in the container
	ndbOperatorImageName := os.Getenv("NDB_OPERATOR_IMAGE")
	container.Image = ndbOperatorImageName
	container.ImagePullPolicy = corev1.PullIfNotPresent

	return []corev1.Container{container}
}

// newStatefulSet defines a new StatefulSet that will be
// used by the mgmd and ndbmtd StatefulSets
func (bss *baseStatefulSet) newStatefulSet(
	nc *v1.NdbCluster, cs *ndbconfig.ConfigSummary) *appsv1.StatefulSet {

	// Fill in the podSpec with any provided ImagePullSecrets
	var podSpec corev1.PodSpec
	imagePullSecretName := nc.Spec.ImagePullSecretName
	if imagePullSecretName != "" {
		podSpec.ImagePullSecrets = append(podSpec.ImagePullSecrets, corev1.LocalObjectReference{
			Name: imagePullSecretName,
		})
	}

	// Add the default init container and add the ndbOperatorImagePullSecretName
	// to the existing ImagePullSecrets list.
	podSpec.InitContainers = bss.getDefaultInitContainers(nc)
	ndbOperatorImagePullSecretName := os.Getenv("NDB_OPERATOR_IMAGE_PULL_SECRET_NAME")
	if ndbOperatorImagePullSecretName != "" {
		podSpec.ImagePullSecrets = append(podSpec.ImagePullSecrets, corev1.LocalObjectReference{
			Name: ndbOperatorImagePullSecretName,
		})
	}

	// Add the empty dir volume
	podSpec.Volumes = []corev1.Volume{*bss.getEmptyDirPodVolume(workDirVolName)}

	podSpec.ServiceAccountName = nc.GetServiceAccountName()

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
func (bss *baseStatefulSet) GetName(nc *v1.NdbCluster) string {
	return nc.GetWorkloadName(bss.nodeType)
}

// GetTypeName returns the type name of baseStatefulSet
func (bss *baseStatefulSet) GetTypeName() constants.NdbNodeType {
	return bss.nodeType
}
