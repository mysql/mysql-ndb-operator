// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"strconv"
	"strings"

	"github.com/mysql/ndb-operator/config/debug"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/ndbconfig"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"
)

const (
	// sfsetTypeMgmd represents the management node type
	sfsetTypeMgmd = "mgmd"
	// sfsetTypeNdbd represents the data node type
	sfsetTypeNdbd = "ndbd"
	// statefulset generated PVC name prefix
	volumeClaimTemplateName = "ndb-pvc"

	// common data directory path for both mgmd and ndbd
	dataDirectoryMountPath = constants.DataDir + "/data"

	// config.ini volume and mount path for the management pods
	mgmdConfigIniVolumeName = sfsetTypeMgmd + "-config-volume"
	mgmdConfigIniMountPath  = constants.DataDir + "/config"

	// Volume name and mount path for data node helper scripts
	dataNodeHelperScriptsVolName   = "datanode-helper-scripts-vol"
	dataNodeHelperScriptsMountPath = constants.DataDir + "/scripts"

	// datanode-healthcheck.sh configmap key
	dataNodeHealthCheckKey = "datanode-healthcheck.sh"
)

// StatefulSetInterface is the interface for a statefulset of NDB management or data nodes
type StatefulSetInterface interface {
	GetTypeName() string
	NewStatefulSet(cs *ndbconfig.ConfigSummary, cluster *v1alpha1.NdbCluster) *apps.StatefulSet
	GetName(nc *v1alpha1.NdbCluster) string
}

type baseStatefulSet struct {
	typeName string
}

// getStatefulSetLabels returns the labels of the StatefulSet
func (bss *baseStatefulSet) getStatefulSetLabels(nc *v1alpha1.NdbCluster) map[string]string {
	return nc.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: bss.typeName + "-statefulset",
	})
}

// getPodLabels generates the labels of the pods controlled by the StatefulSets
func (bss *baseStatefulSet) getPodLabels(nc *v1alpha1.NdbCluster) map[string]string {
	return nc.GetCompleteLabels(map[string]string{
		constants.ClusterNodeTypeLabel: bss.typeName,
	})
}

// getEmptyDirVolumeName returns the name of the empty directory volume
func (bss *baseStatefulSet) getEmptyDirVolumeName() string {
	return bss.typeName + "-empty-dir-volume"
}

// getEmptyDirPodVolumes returns an empty directory pod volume
func (bss *baseStatefulSet) getEmptyDirPodVolume() *v1.Volume {
	return &v1.Volume{
		Name: bss.getEmptyDirVolumeName(),
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}
}

// createContainers creates a new container for the stateful set.
func (bss *baseStatefulSet) createContainers(
	nc *v1alpha1.NdbCluster, commandAndArgs []string, volumeMounts []v1.VolumeMount,
	startupProbe, readinessProbe *v1.Probe) []v1.Container {

	if debug.Enabled {
		// Increase verbosity
		commandAndArgs = append(commandAndArgs, "-v")
	}

	klog.Infof("Creating %q container from image %s", bss.typeName, nc.Spec.Image)
	return []v1.Container{
		{
			Name: bss.GetTypeName() + "-container",
			// Use the image provided in spec
			Image:           nc.Spec.Image,
			ImagePullPolicy: nc.Spec.ImagePullPolicy,
			// Expose the port 1186 for both mgmd and ndbd containers
			Ports: []v1.ContainerPort{
				{
					ContainerPort: 1186,
				},
			},
			VolumeMounts:   volumeMounts,
			Command:        []string{"/bin/bash", "-ecx", strings.Join(commandAndArgs, " ")},
			StartupProbe:   startupProbe,
			ReadinessProbe: readinessProbe,
		},
	}
}

// newStatefulSet defines a new StatefulSet that will be
// used by the mgmd and ndbd StatefulSets
func (bss *baseStatefulSet) newStatefulSet(nc *v1alpha1.NdbCluster) *apps.StatefulSet {

	// Fill in the podSpec with any provided ImagePullSecrets
	var podSpec v1.PodSpec
	imagePullSecretName := nc.Spec.ImagePullSecretName
	if imagePullSecretName != "" {
		podSpec.ImagePullSecrets = []v1.LocalObjectReference{
			{
				Name: imagePullSecretName,
			},
		}
	}

	// Labels to be used for the statefulset pods
	podLabels := bss.getPodLabels(nc)

	return &apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:   bss.GetName(nc),
			Labels: bss.getStatefulSetLabels(nc),
			// Owner reference pointing to the Ndb resource
			OwnerReferences: nc.GetOwnerReferences(),
		},
		Spec: apps.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				// Use the pods labels as the selector
				MatchLabels: podLabels,
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
				},
				Spec: podSpec,
			},
			// The services must exist before the StatefulSet,
			// and is responsible for the network identity of the set.
			ServiceName: nc.GetServiceName(bss.typeName),
		},
	}
}

// GetName returns the name of the baseStatefulSet
func (bss *baseStatefulSet) GetName(nc *v1alpha1.NdbCluster) string {
	return nc.Name + "-" + bss.typeName
}

// GetTypeName returns the type name of baseStatefulSet
func (bss *baseStatefulSet) GetTypeName() string {
	return bss.typeName
}

// mgmdStatefulSet implements the StatefulSetInterface to control a set of management nodes
type mgmdStatefulSet struct {
	baseStatefulSet
}

// getPodVolumes returns a slice of volumes to be
// made available to the management server pods.
func (mss *mgmdStatefulSet) getPodVolumes(nc *v1alpha1.NdbCluster) []v1.Volume {

	// Management Server pods have an empty directory volume
	// for storing local config cache and a config map
	// volume that has the MySQL Cluster configuration.
	var podVolumes []v1.Volume
	podVolumes = append(podVolumes, *mss.getEmptyDirPodVolume())

	// Append the configmap's config.ini as a volume to the Management pods
	podVolumes = append(podVolumes, v1.Volume{
		Name: mgmdConfigIniVolumeName,
		VolumeSource: v1.VolumeSource{
			ConfigMap: &v1.ConfigMapVolumeSource{
				LocalObjectReference: v1.LocalObjectReference{
					Name: nc.GetConfigMapName(),
				},
				// Load only the config.ini key
				Items: []v1.KeyToPath{
					{
						Key:  constants.ConfigIniKey,
						Path: constants.ConfigIniKey,
					},
				},
			},
		},
	})

	return podVolumes
}

// getVolumeMounts returns the volumes to be mounted to the mgmd containers
func (mss *mgmdStatefulSet) getVolumeMounts() []v1.VolumeMount {

	var volumeMounts []v1.VolumeMount

	// Append the empty dir volume mount to be used as a config directory
	volumeMounts = append(volumeMounts, v1.VolumeMount{
		Name:      mss.getEmptyDirVolumeName(),
		MountPath: dataDirectoryMountPath,
	})

	// Mount the config map volume holding the MySQL Cluster configuration
	volumeMounts = append(volumeMounts, v1.VolumeMount{
		Name:      mgmdConfigIniVolumeName,
		MountPath: mgmdConfigIniMountPath,
	})

	return volumeMounts
}

// getContainers returns the containers to run a Management Node
func (mss *mgmdStatefulSet) getContainers(nc *v1alpha1.NdbCluster) []v1.Container {

	// Command and args to run the management server
	cmdAndArgs := []string{
		"/usr/sbin/ndb_mgmd",
		"-f", mgmdConfigIniMountPath + "/config.ini",
		"--initial",
		"--nodaemon",
		"--config-cache=0",
	}

	volumeMounts := mss.getVolumeMounts()

	// Health probe handler for management node
	portCheckHandler := v1.Handler{
		TCPSocket: &v1.TCPSocketAction{
			Port: intstr.FromInt(1186),
		},
	}

	// Startup probe - expects mgmd to get ready within a minute
	startupProbe := &v1.Probe{
		Handler:          portCheckHandler,
		PeriodSeconds:    1,
		FailureThreshold: 60,
	}

	// Readiness probe
	readinessProbe := &v1.Probe{
		Handler: portCheckHandler,
	}

	return mss.createContainers(nc, cmdAndArgs, volumeMounts, startupProbe, readinessProbe)
}

func (mss *mgmdStatefulSet) getPodAntiAffinity() *v1.PodAntiAffinity {
	// Default pod AntiAffinity rules for Management Nodes
	return getPodAntiAffinityRules([]string{
		mysqldClientName, sfsetTypeNdbd, sfsetTypeMgmd,
	})
}

// NewStatefulSet returns the StatefulSet specification to start and manage the Management nodes.
func (mss *mgmdStatefulSet) NewStatefulSet(cs *ndbconfig.ConfigSummary, nc *v1alpha1.NdbCluster) *apps.StatefulSet {
	statefulSet := mss.newStatefulSet(nc)
	statefulSetSpec := &statefulSet.Spec

	// Fill in mgmd specific values
	replicas := cs.NumOfManagementNodes
	statefulSetSpec.Replicas = &replicas
	// Set pod management policy to start Management nodes one by one
	statefulSetSpec.PodManagementPolicy = apps.OrderedReadyPodManagement

	// Update template pod spec
	podSpec := &statefulSetSpec.Template.Spec
	podSpec.Containers = mss.getContainers(nc)
	podSpec.Volumes = mss.getPodVolumes(nc)
	// Set default AntiAffinity rules
	podSpec.Affinity = &v1.Affinity{
		PodAntiAffinity: mss.getPodAntiAffinity(),
	}
	// Copy down any podSpec specified via CRD
	copyPodSpecFromNdbPodSpec(podSpec, nc.Spec.ManagementNodePodSpec)

	return statefulSet
}

// NewMgmdStatefulSet returns a new StatefulSetInterface for management nodes
func NewMgmdStatefulSet() StatefulSetInterface {
	return &mgmdStatefulSet{
		baseStatefulSet{
			typeName: sfsetTypeMgmd,
		},
	}
}

// ndbdStatefulSet implements the StatefulSetInterface to control a set of data nodes
type ndbdStatefulSet struct {
	baseStatefulSet
}

// getPodVolumes returns a slice of volumes to be
// made available to the data node pods.
func (nss *ndbdStatefulSet) getPodVolumes(nc *v1alpha1.NdbCluster) []v1.Volume {

	// Load the data node scripts from
	// the configmap into the pod via a volume
	podVolumes := []v1.Volume{
		{
			Name: dataNodeHelperScriptsVolName,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: nc.GetConfigMapName(),
					},
					Items: []v1.KeyToPath{
						{
							Key:  dataNodeHealthCheckKey,
							Path: dataNodeHealthCheckKey,
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

// getVolumeMounts returns the volumes to be mounted to the ndbd containers
func (nss *ndbdStatefulSet) getVolumeMounts(nc *v1alpha1.NdbCluster) []v1.VolumeMount {

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
	return []v1.VolumeMount{
		{
			// Volume mount for data directory
			Name:      dataDirVolumeName,
			MountPath: dataDirectoryMountPath,
		},
		{
			// Volume mount for helper scripts
			Name:      dataNodeHelperScriptsVolName,
			MountPath: dataNodeHelperScriptsMountPath,
		},
	}
}

// getContainers returns the containers to run a data Node
func (nss *ndbdStatefulSet) getContainers(nc *v1alpha1.NdbCluster) []v1.Container {

	// Command and args to run the management server
	cmdAndArgs := []string{
		"/usr/sbin/ndbmtd",
		"-c", nc.GetConnectstring(),
		"--foreground",
	}

	volumeMounts := nss.getVolumeMounts(nc)

	// Setup startup probe for data nodes.
	// The probe uses a script that checks if a data node has started, by
	// connecting to the Management node via ndb_mgm. This implies that atleast
	// one Management node has to be available for the probe to succeed. This
	// is fine as that is already a requirement for a data node to start. Even
	// if the Management node crashes or becomes unavailable when the data node
	// is going through the start phases, the Management node will be
	// rescheduled immediately and will become ready within a few seconds,
	// enabling the data node startup probe to succeed.
	dataNodeStartNodeId := nc.GetManagementNodeCount() + 1
	startupProbe := &v1.Probe{
		Handler: v1.Handler{
			Exec: &v1.ExecAction{
				// datanode-healthcheck.sh <mgmd service> <data node start node id>
				Command: []string{
					"/bin/bash",
					dataNodeHelperScriptsMountPath + "/" + dataNodeHealthCheckKey,
					nc.GetConnectstring(),
					strconv.Itoa(int(dataNodeStartNodeId)),
				},
			},
		},
		// expect data node to get ready within 15 minutes
		PeriodSeconds:    2,
		TimeoutSeconds:   2,
		FailureThreshold: 450,
	}

	return nss.createContainers(nc, cmdAndArgs, volumeMounts, startupProbe, nil)
}

func (nss *ndbdStatefulSet) getPodAntiAffinity() *v1.PodAntiAffinity {
	// Default pod AntiAffinity rules for Data Nodes
	return getPodAntiAffinityRules([]string{
		sfsetTypeMgmd, mysqldClientName, sfsetTypeNdbd,
	})
}

// NewStatefulSet returns the StatefulSet specification to start and manage the Data nodes.
func (nss *ndbdStatefulSet) NewStatefulSet(cs *ndbconfig.ConfigSummary, nc *v1alpha1.NdbCluster) *apps.StatefulSet {
	statefulSet := nss.newStatefulSet(nc)
	statefulSetSpec := &statefulSet.Spec

	// Fill in ndbd specific values
	replicas := cs.NumOfDataNodes
	statefulSetSpec.Replicas = &replicas
	// Set pod management policy to start Data nodes in parallel
	statefulSetSpec.PodManagementPolicy = apps.ParallelPodManagement

	// Add VolumeClaimTemplate if data node PVC Spec exists
	if nc.Spec.DataNodePVCSpec != nil {
		statefulSetSpec.VolumeClaimTemplates = []v1.PersistentVolumeClaim{
			// This PVC will be used as a template and an actual PVC will be created by the
			// statefulset controller with name "<volumeClaimTemplateName>-<ndb-name>-<pod-name>"
			*NewPVC(nc, volumeClaimTemplateName, nc.Spec.DataNodePVCSpec),
		}
	}

	// Update template pod spec
	podSpec := &statefulSetSpec.Template.Spec
	podSpec.Containers = nss.getContainers(nc)
	podSpec.Volumes = nss.getPodVolumes(nc)
	// Set default AntiAffinity rules
	podSpec.Affinity = &v1.Affinity{
		PodAntiAffinity: nss.getPodAntiAffinity(),
	}
	// Copy down any podSpec specified via CRD
	copyPodSpecFromNdbPodSpec(podSpec, nc.Spec.DataNodePodSpec)

	return statefulSet
}

// NewNdbdStatefulSet returns a new StatefulSetInterface for data nodes
func NewNdbdStatefulSet() StatefulSetInterface {
	return &ndbdStatefulSet{
		baseStatefulSet{
			typeName: sfsetTypeNdbd,
		},
	}
}
