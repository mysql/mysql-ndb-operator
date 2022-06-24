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
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// config.ini volume and mount path for the management pods
	mgmdConfigIniVolumeName = constants.NdbNodeTypeMgmd + "-config-volume"
	mgmdConfigIniMountPath  = constants.DataDir + "/config"
)

// mgmdStatefulSet implements the NdbStatefulSetInterface to control a set of management nodes
type mgmdStatefulSet struct {
	baseStatefulSet
}

// getPodVolumes returns a slice of volumes to be
// made available to the management server pods.
func (mss *mgmdStatefulSet) getPodVolumes(nc *v1alpha1.NdbCluster) []corev1.Volume {

	return []corev1.Volume{
		// Empty Dir volume for the mgmd data dir
		*mss.getEmptyDirPodVolume(),

		// Load the config.ini script via a volume
		{
			Name: mgmdConfigIniVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: nc.GetConfigMapName(),
					},
					// Load only the config.ini key
					Items: []corev1.KeyToPath{
						{
							Key:  constants.ConfigIniKey,
							Path: constants.ConfigIniKey,
						},
					},
				},
			},
		},

		// Load the helper scripts from
		// the configmap into the pod via a volume
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
							// Load the wait-for-dns-update script.
							// It will be used by the init container.
							Key:  constants.WaitForDNSUpdateScript,
							Path: constants.WaitForDNSUpdateScript,
						},
						{
							// Load the startup probe
							Key:  constants.MgmdStartupProbeScript,
							Path: constants.MgmdStartupProbeScript,
						},
					},
				},
			},
		},
	}
}

// getVolumeMounts returns the volumes to be mounted to the mgmd containers
func (mss *mgmdStatefulSet) getVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		// Append the empty dir volume mount to be used as a data dir
		{
			Name:      mss.getEmptyDirVolumeName(),
			MountPath: dataDirectoryMountPath,
		},
		// Mount the config map volume holding the MySQL Cluster configuration
		{
			Name:      mgmdConfigIniVolumeName,
			MountPath: mgmdConfigIniMountPath,
		},
		// Mount the helper scripts
		mss.getHelperScriptVolumeMount(),
	}
}

// getInitContainers returns the init containers to be used by the management Node
func (mss *mgmdStatefulSet) getInitContainers(nc *v1alpha1.NdbCluster) []corev1.Container {
	// Command and args to run the mgmd init script
	cmdAndArgs := []string{
		helperScriptsMountPath + "/" + constants.WaitForDNSUpdateScript,
	}

	return []corev1.Container{
		mss.createContainer(nc,
			mss.getContainerName(true),
			cmdAndArgs, mss.getVolumeMounts()),
	}
}

// getContainers returns the containers to run a Management Node
func (mss *mgmdStatefulSet) getContainers(nc *v1alpha1.NdbCluster) []corev1.Container {

	// Command and args to run the management server
	cmdAndArgs := []string{
		"/usr/sbin/ndb_mgmd",
		"-f", mgmdConfigIniMountPath + "/config.ini",
		"--initial",
		"--nodaemon",
		"--config-cache=0",
	}

	if debug.Enabled {
		// Increase verbosity in debug mode
		cmdAndArgs = append(cmdAndArgs, "-v")
	}

	mgmdContainer := mss.createContainer(nc,
		mss.getContainerName(false),
		cmdAndArgs, mss.getVolumeMounts(), 1186)

	// Startup probe for the mgmd container
	mgmdContainer.StartupProbe = &corev1.Probe{
		Handler: corev1.Handler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"/bin/bash",
					helperScriptsMountPath + "/" + constants.MgmdStartupProbeScript,
				},
			},
		},
		// Startup probe - expects mgmd to get ready within a minute
		PeriodSeconds:    1,
		TimeoutSeconds:   3,
		FailureThreshold: 60,
	}

	// Readiness probe checks if the port 1186 is open
	mgmdContainer.ReadinessProbe = &corev1.Probe{
		Handler: corev1.Handler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt(1186),
			},
		},
	}

	return []corev1.Container{mgmdContainer}
}

func (mss *mgmdStatefulSet) getPodAntiAffinity() *corev1.PodAntiAffinity {
	// Default pod AntiAffinity rules for Management Nodes
	return GetPodAntiAffinityRules([]string{
		constants.NdbNodeTypeMySQLD, constants.NdbNodeTypeNdbmtd, constants.NdbNodeTypeMgmd,
	})
}

// NewStatefulSet returns the StatefulSet specification to start and manage the Management nodes.
func (mss *mgmdStatefulSet) NewStatefulSet(cs *ndbconfig.ConfigSummary, nc *v1alpha1.NdbCluster) *appsv1.StatefulSet {
	statefulSet := mss.newStatefulSet(nc, cs)
	statefulSetSpec := &statefulSet.Spec

	// Fill in mgmd specific values
	replicas := cs.NumOfManagementNodes
	statefulSetSpec.Replicas = &replicas
	// Set pod management policy to start Management nodes one by one
	statefulSetSpec.PodManagementPolicy = appsv1.OrderedReadyPodManagement

	// Update template pod spec
	podSpec := &statefulSetSpec.Template.Spec
	podSpec.InitContainers = mss.getInitContainers(nc)
	podSpec.Containers = mss.getContainers(nc)
	podSpec.Volumes = mss.getPodVolumes(nc)
	// Set default AntiAffinity rules
	podSpec.Affinity = &corev1.Affinity{
		PodAntiAffinity: mss.getPodAntiAffinity(),
	}
	// Copy down any podSpec specified via CRD
	CopyPodSpecFromNdbPodSpec(podSpec, nc.Spec.ManagementNodePodSpec)

	return statefulSet
}

// NewMgmdStatefulSet returns a new NdbStatefulSetInterface for management nodes
func NewMgmdStatefulSet() NdbStatefulSetInterface {
	return &mgmdStatefulSet{
		baseStatefulSet{
			nodeType: constants.NdbNodeTypeMgmd,
		},
	}
}
