// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"fmt"
	"strings"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

const (
	// sfsetTypeMgmd represents the management node type
	sfsetTypeMgmd = "mgmd"
	// sfsetTypeNdbd represents the data node type
	sfsetTypeNdbd = "ndbd"
	// statefulset generated PVC name prefix
	volumeClaimTemplateName = "ndb-pvc"
)

// StatefulSetInterface is the interface for a statefulset of NDB management or data nodes
type StatefulSetInterface interface {
	GetTypeName() string
	NewStatefulSet(rc *ResourceContext, cluster *v1alpha1.Ndb) *apps.StatefulSet
	GetName() string
}

type baseStatefulSet struct {
	typeName    string
	clusterName string
}

// NewMgmdStatefulSet returns a new baseStatefulSet for management nodes
func NewMgmdStatefulSet(cluster *v1alpha1.Ndb) *baseStatefulSet {
	return &baseStatefulSet{typeName: sfsetTypeMgmd, clusterName: cluster.Name}
}

// NewNdbdStatefulSet returns a new baseStatefulSet for data nodes
func NewNdbdStatefulSet(cluster *v1alpha1.Ndb) *baseStatefulSet {
	return &baseStatefulSet{typeName: sfsetTypeNdbd, clusterName: cluster.Name}
}

// isMgmd returns if the baseStatefulSet represents a management node
func (bss *baseStatefulSet) isMgmd() bool {
	return bss.typeName == sfsetTypeMgmd
}

// isMgmd returns if the baseStatefulSet represents a data node
func (bss *baseStatefulSet) isNdbd() bool {
	return bss.typeName == sfsetTypeNdbd
}

// getEmptyDirectoryVolumeName returns the name of the empty directory volume
func (bss *baseStatefulSet) getEmptyDirVolumeName() string {
	return bss.clusterName + bss.typeName + "-volume"
}

// getPodVolumes returns the named volume available in the Pods
func (bss *baseStatefulSet) getPodVolumes(ndb *v1alpha1.Ndb) []v1.Volume {

	var podVolumes []v1.Volume
	if bss.isMgmd() || ndb.Spec.DataNodePVCSpec == nil {
		// Use an empty directory volume if,
		// (a) this is a mgmd pod and it will use this volume to store config (or)
		// (b) this is a ndbd pod whose PVC Spec was not specified as a part
		//     of Ndb spec and it will use this volume as the data directory
		podVolumes = append(podVolumes, v1.Volume{
			Name: bss.getEmptyDirVolumeName(),
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		})
	}

	if bss.isMgmd() {
		// Add the configmap's config.ini as a volume to the Management pods
		podVolumes = append(podVolumes, v1.Volume{
			Name: "config-volume",
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: ndb.GetConfigMapName(),
					},
					// Load only the config.ini key
					Items: []v1.KeyToPath{
						{
							Key:  configIniKey,
							Path: configIniKey,
						},
					},
				},
			},
		})
	}

	return podVolumes
}

// getVolumeMounts returns the volumes to be mounted to the container
func (bss *baseStatefulSet) getVolumeMounts(ndb *v1alpha1.Ndb) []v1.VolumeMount {

	var volumeName string
	if bss.isNdbd() && ndb.Spec.DataNodePVCSpec != nil {
		// Use the volumeClaimTemplate name for the data node
		volumeName = volumeClaimTemplateName
	} else {
		volumeName = bss.getEmptyDirVolumeName()
	}

	// volume mount for the data directory/config directory
	volumeMounts := []v1.VolumeMount{
		{
			Name:      volumeName,
			MountPath: constants.DataDir,
		},
	}

	if bss.isMgmd() {
		// Mount the config map volume holding the
		// cluster configuration into the management container
		volumeMounts = append(volumeMounts, v1.VolumeMount{
			Name:      "config-volume",
			MountPath: constants.DataDir + "/config",
		})
	}

	return volumeMounts
}

// GetName returns the name of the baseStatefulSet
func (bss *baseStatefulSet) GetName() string {
	return bss.clusterName + "-" + bss.typeName
}

// GetTypeName returns the type name of baseStatefulSet
func (bss *baseStatefulSet) GetTypeName() string {
	return bss.typeName
}

// getStatefulSetLabels returns the labels of the statefulset
func (bss *baseStatefulSet) getStatefulSetLabels(ndb *v1alpha1.Ndb) map[string]string {
	return ndb.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: bss.typeName + "-statefulset",
	})
}

// getPodLabels generates the labels of the pods controlled by the statefulsets
func (bss *baseStatefulSet) getPodLabels(ndb *v1alpha1.Ndb) map[string]string {
	return ndb.GetCompleteLabels(map[string]string{
		constants.ClusterNodeTypeLabel: bss.typeName,
	})
}

// getContainers returns the container to run the NDB node represented by the baseStatefulSet
func (bss *baseStatefulSet) getContainers(ndb *v1alpha1.Ndb) []v1.Container {
	// Set type specific values
	var cmd string
	var args []string
	if bss.isMgmd() {
		// Mgmd specific details
		cmd = "/usr/sbin/ndb_mgmd"
		args = []string{
			"-f", "/var/lib/ndb/config/config.ini",
			"--configdir=" + constants.DataDir,
			"--initial",
			"--nodaemon",
			"--config-cache=0",
			"-v",
		}
	} else {
		// Ndbd specific details
		cmd = "/usr/sbin/ndbmtd"
		args = []string{
			"-c", ndb.GetConnectstring(),
			"--nodaemon",
			"-v",
		}
	}

	// Build the command
	cmdArgs := fmt.Sprintf("%s %s", cmd, strings.Join(args, " "))

	// Use the image provided in spec
	imageName := ndb.Spec.ContainerImage
	klog.Infof("Creating %s container from image %s", bss.typeName, imageName)

	return []v1.Container{
		{
			Name:  bss.GetTypeName() + "-container",
			Image: imageName,
			Ports: []v1.ContainerPort{
				{
					ContainerPort: 1186,
				},
			},
			VolumeMounts:    bss.getVolumeMounts(ndb),
			Command:         []string{"/bin/bash", "-ecx", cmdArgs},
			ImagePullPolicy: v1.PullIfNotPresent,
		},
	}
}

// NewStatefulSet creates a new StatefulSet for the given Cluster.
func (bss *baseStatefulSet) NewStatefulSet(rc *ResourceContext, ndb *v1alpha1.Ndb) *apps.StatefulSet {

	// Build the new stateful set and return
	podLabels := bss.getPodLabels(ndb)
	var podManagementPolicy apps.PodManagementPolicyType
	var replicas int32
	var volumeClaimTemplates []v1.PersistentVolumeClaim
	if bss.isMgmd() {
		replicas = int32(rc.ManagementNodeCount)
		// Start Management nodes one by one
		podManagementPolicy = apps.OrderedReadyPodManagement
	} else {
		replicas = int32(rc.GetDataNodeCount())
		// Data nodes can be started in parallel
		podManagementPolicy = apps.ParallelPodManagement
		// Add VolumeClaimTemplate if data node PVC Spec exists
		if ndb.Spec.DataNodePVCSpec != nil {
			volumeClaimTemplates = []v1.PersistentVolumeClaim{
				// This PVC will be used as a template and an actual PVC will be created by the
				// statefulset controller with name "<volumeClaimTemplateName>-<ndb-name>-<pod-name>"
				*NewPVC(ndb, volumeClaimTemplateName, ndb.Spec.DataNodePVCSpec),
			}
		}
	}

	ss := &apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   bss.GetName(),
			Labels: bss.getStatefulSetLabels(ndb),
			// Owner reference pointing to the Ndb resource
			OwnerReferences: ndb.GetOwnerReferences(),
		},
		Spec: apps.StatefulSetSpec{
			UpdateStrategy: apps.StatefulSetUpdateStrategy{
				// we want to be able to control ndbd node restarts directly
				Type: apps.OnDeleteStatefulSetStrategyType,
			},
			Selector: &metav1.LabelSelector{
				// must match templates labels
				MatchLabels: podLabels,
			},
			Replicas: &replicas,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:        bss.GetName(),
					Labels:      podLabels,
					Annotations: map[string]string{},
				},
				Spec: v1.PodSpec{
					Containers:         bss.getContainers(ndb),
					Volumes:            bss.getPodVolumes(ndb),
					ServiceAccountName: "ndb-agent",
				},
			},
			// service must exist before the StatefulSet, and is responsible for
			// the network identity of the set.
			ServiceName:          ndb.GetServiceName(bss.typeName),
			PodManagementPolicy:  podManagementPolicy,
			VolumeClaimTemplates: volumeClaimTemplates,
		},
	}
	return ss
}
