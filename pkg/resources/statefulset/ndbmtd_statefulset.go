// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package statefulset

import (
	"github.com/mysql/ndb-operator/config/debug"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/ndbconfig"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	// statefulset generated PVC name prefix
	volumeClaimTemplateName = "ndb-pvc"
)

var (
	// Ports to be exposed by the container and service
	ndbmtdPorts = []int32{1186}
)

// ndbmtdStatefulSet implements the NdbStatefulSetInterface to control a set of data nodes
type ndbmtdStatefulSet struct {
	baseStatefulSet
}

// getPodVolumes returns a slice of volumes to be
// made available to the data node pods.
func (nss *ndbmtdStatefulSet) getPodVolumes(nc *v1alpha1.NdbCluster) []corev1.Volume {

	// Load the data node scripts from
	// the configmap into the pod via a volume
	podVolumes := []corev1.Volume{
		{
			Name: helperScriptsVolName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: nc.GetConfigMapName(),
					},
					DefaultMode: &ownerCanExecMode,
					Items: []corev1.KeyToPath{
						{
							Key:  constants.DataNodeStartupProbeScript,
							Path: constants.DataNodeStartupProbeScript,
						},
						{
							Key:  constants.DataNodeInitScript,
							Path: constants.DataNodeInitScript,
						},
						{
							// Load the wait-for-dns-update script.
							// It will be used by the init container.
							Key:  constants.WaitForDNSUpdateScript,
							Path: constants.WaitForDNSUpdateScript,
						},
					},
				},
			},
		},
	}

	// An empty directory volume needs to be provided
	// to the data node pods if the NdbCluster resource
	// doesn't have any PVCs defined to be used with
	// the data nodes.
	if nc.Spec.DataNodePVCSpec == nil {
		podVolumes = append(podVolumes, *nss.getEmptyDirPodVolume())
	}

	return podVolumes
}

// getVolumeMounts returns the volumes to be mounted to the ndbmtd containers
func (nss *ndbmtdStatefulSet) getVolumeMounts(nc *v1alpha1.NdbCluster) []corev1.VolumeMount {

	var dataDirVolumeName string
	if nc.Spec.DataNodePVCSpec == nil {
		// NdbCluster doesn't have a PVC spec defined for the data nodes.
		// Use the empty dir volume mount as the data directory
		dataDirVolumeName = nss.getEmptyDirVolumeName()
	} else {
		// Use the volumeClaimTemplate name for the data node
		dataDirVolumeName = volumeClaimTemplateName
	}

	// return volume mounts
	return []corev1.VolumeMount{
		{
			// Volume mount for data directory
			Name:      dataDirVolumeName,
			MountPath: dataDirectoryMountPath,
		},
		// Volume mount for helper scripts
		nss.getHelperScriptVolumeMount(),
	}
}

// getInitContainers returns the init containers to be used by the data Node
func (nss *ndbmtdStatefulSet) getInitContainers(nc *v1alpha1.NdbCluster) []corev1.Container {
	// Command and args to run the Data node init script
	cmdAndArgs := []string{
		helperScriptsMountPath + "/" + constants.DataNodeInitScript,
		nc.GetConnectstring(),
	}

	return []corev1.Container{
		nss.createContainer(nc,
			nss.getContainerName(true),
			cmdAndArgs, nss.getVolumeMounts(nc), nil),
	}
}

// getContainers returns the containers to run a data Node
func (nss *ndbmtdStatefulSet) getContainers(nc *v1alpha1.NdbCluster) []corev1.Container {

	// Command and args to run the Data node
	cmdAndArgs := []string{
		"/usr/sbin/ndbmtd",
		"-c", nc.GetConnectstring(),
		"--foreground",
		// Pass the nodeId to be used to prevent invalid
		// nodeId allocation during statefulset patching.
		"--ndb-nodeid=$(cat " + dataNodeIdFilePath + ")",
	}

	if debug.Enabled {
		// Increase verbosity in debug mode
		cmdAndArgs = append(cmdAndArgs, "-v")
	}

	ndbmtdContainer := nss.createContainer(
		nc, nss.getContainerName(false), cmdAndArgs,
		nss.getVolumeMounts(nc), ndbmtdPorts)

	// Setup startup probe for data nodes.
	// The probe uses a script that checks if a data node has started, by
	// connecting to the Management node via ndb_mgm. This implies that atleast
	// one Management node has to be available for the probe to succeed. This
	// is fine as that is already a requirement for a data node to start. Even
	// if the Management node crashes or becomes unavailable when the data node
	// is going through the start phases, the Management node will be
	// rescheduled immediately and will become ready within a few seconds,
	// enabling the data node startup probe to succeed.
	ndbmtdContainer.StartupProbe = &corev1.Probe{
		Handler: corev1.Handler{
			Exec: &corev1.ExecAction{
				// datanode-startup-probe.sh
				Command: []string{
					"/bin/bash",
					helperScriptsMountPath + "/" + constants.DataNodeStartupProbeScript,
				},
			},
		},
		// expect data node to get ready within 15 minutes
		PeriodSeconds:    2,
		TimeoutSeconds:   2,
		FailureThreshold: 450,
	}

	return []corev1.Container{ndbmtdContainer}
}

func (nss *ndbmtdStatefulSet) getPodAntiAffinity() *corev1.PodAntiAffinity {
	// Default pod AntiAffinity rules for Data Nodes
	return GetPodAntiAffinityRules([]constants.NdbNodeType{
		constants.NdbNodeTypeMgmd, constants.NdbNodeTypeMySQLD, constants.NdbNodeTypeNdbmtd,
	})
}

// NewStatefulSet returns the StatefulSet specification to start and manage the Data nodes.
func (nss *ndbmtdStatefulSet) NewStatefulSet(cs *ndbconfig.ConfigSummary, nc *v1alpha1.NdbCluster) *appsv1.StatefulSet {
	statefulSet := nss.newStatefulSet(nc, cs)
	statefulSetSpec := &statefulSet.Spec

	// Fill in ndbmtd specific values
	replicas := cs.NumOfDataNodes
	statefulSetSpec.Replicas = &replicas
	// Set pod management policy to start Data nodes in parallel
	statefulSetSpec.PodManagementPolicy = appsv1.ParallelPodManagement

	// Use the legacy OnDelete update strategy to get more
	// control over how the update is rolled out to data nodes
	statefulSetSpec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.OnDeleteStatefulSetStrategyType,
	}

	// Add VolumeClaimTemplate if data node PVC Spec exists
	if nc.Spec.DataNodePVCSpec != nil {
		statefulSetSpec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			// This PVC will be used as a template and an actual PVC will be created by the
			// statefulset controller with name "<volumeClaimTemplateName>-<ndb-name>-<pod-name>"
			*newPVC(nc, volumeClaimTemplateName, nc.Spec.DataNodePVCSpec),
		}
	}

	// Update template pod spec
	podSpec := &statefulSetSpec.Template.Spec
	podSpec.InitContainers = nss.getInitContainers(nc)
	podSpec.Containers = nss.getContainers(nc)
	podSpec.Volumes = nss.getPodVolumes(nc)
	// Set default AntiAffinity rules
	podSpec.Affinity = &corev1.Affinity{
		PodAntiAffinity: nss.getPodAntiAffinity(),
	}
	// Copy down any podSpec specified via CRD
	CopyPodSpecFromNdbPodSpec(podSpec, nc.Spec.DataNodePodSpec)

	return statefulSet
}

// NewNdbmtdStatefulSet returns a new NdbStatefulSetInterface for data nodes
func NewNdbmtdStatefulSet() NdbStatefulSetInterface {
	return &ndbmtdStatefulSet{
		baseStatefulSet{
			nodeType: constants.NdbNodeTypeNdbmtd,
		},
	}
}
