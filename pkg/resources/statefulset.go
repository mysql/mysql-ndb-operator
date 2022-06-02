// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
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
	// dataNodeIdFilePath is the location of the file that has the data node's nodeId
	dataNodeIdFilePath = dataDirectoryMountPath + "/nodeId.val"

	// Common volume name and mount path for data node and mgmd node helper scripts
	helperScriptsVolName   = "helper-scripts-vol"
	helperScriptsMountPath = constants.DataDir + "/scripts"

	// config.ini volume and mount path for the management pods
	mgmdConfigIniVolumeName = sfsetTypeMgmd + "-config-volume"
	mgmdConfigIniMountPath  = constants.DataDir + "/config"

	// datanode-startup-probe.sh configmap key
	dataNodeStartupProbeKey = "datanode-startup-probe.sh"

	// datanode init container script key
	dataNodeInitScriptKey = "datanode-init-script.sh"

	// Configmap key for the DNS update waiter script
	waitForDNSUpdateScriptKey = "wait-for-dns-update.sh"

	// Management node startup probe script key
	mgmdStartupProbeKey = "mgmd-startup-probe.sh"
)

// Permissions to be set to the helper scripts loaded through configmap
var ownerCanExecMode = int32(0744)

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

// getHelperScriptVolumeMount returns the VolumeMount for the helper scripts
func (bss *baseStatefulSet) getHelperScriptVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		// Volume mount for helper scripts
		Name:      helperScriptsVolName,
		MountPath: helperScriptsMountPath,
	}
}

// createContainers creates a new container for the stateful set.
func (bss *baseStatefulSet) createContainers(
	nc *v1alpha1.NdbCluster, commandAndArgs []string, volumeMounts []v1.VolumeMount,
	startupProbe, readinessProbe *v1.Probe, initContainer bool) []v1.Container {

	var containerName string
	var ports []v1.ContainerPort

	if initContainer {
		containerName = bss.GetTypeName() + "-init-container"
		klog.Infof("Creating %q init-container", bss.typeName)
	} else {
		containerName = bss.GetTypeName() + "-container"
		// Expose the port 1186 for both mgmd and ndbd containers
		ports = []v1.ContainerPort{
			{
				ContainerPort: 1186,
			},
		}
		if debug.Enabled {
			// Increase verbosity
			commandAndArgs = append(commandAndArgs, "-v")
		}
		klog.Infof("Creating %q container from image %s", bss.typeName, nc.Spec.Image)
	}

	return []v1.Container{
		{
			Name: containerName,
			// Use the image provided in spec
			Image:           nc.Spec.Image,
			ImagePullPolicy: nc.Spec.ImagePullPolicy,
			// Expose the port 1186 for both mgmd and ndbd containers
			Ports: ports,
			// Export the Pod IP, Namespace and connectstring to Pod env
			Env: []v1.EnvVar{
				{
					Name: "NDB_POD_IP",
					ValueFrom: &v1.EnvVarSource{
						FieldRef: &v1.ObjectFieldSelector{
							FieldPath: "status.podIP",
						},
					},
				},
				{
					Name: "NDB_POD_NAMESPACE",
					ValueFrom: &v1.EnvVarSource{
						FieldRef: &v1.ObjectFieldSelector{
							FieldPath: "metadata.namespace",
						},
					},
				},
				{
					Name:  "NDB_CONNECTSTRING",
					Value: nc.GetConnectstring(),
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
func (bss *baseStatefulSet) newStatefulSet(
	nc *v1alpha1.NdbCluster, cs *ndbconfig.ConfigSummary) *apps.StatefulSet {

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

	return []v1.Volume{
		// Empty Dir volume for the mgmd data dir
		*mss.getEmptyDirPodVolume(),

		// Load the config.ini script via a volume
		{
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
		},

		// Load the helper scripts from
		// the configmap into the pod via a volume
		{
			Name: helperScriptsVolName,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: nc.GetConfigMapName(),
					},
					DefaultMode: &ownerCanExecMode,
					Items: []v1.KeyToPath{
						{
							// Load the wait-for-dns-update script.
							// It will be used by the init container.
							Key:  waitForDNSUpdateScriptKey,
							Path: waitForDNSUpdateScriptKey,
						},
						{
							// Load the startup probe
							Key:  mgmdStartupProbeKey,
							Path: mgmdStartupProbeKey,
						},
					},
				},
			},
		},
	}
}

// getVolumeMounts returns the volumes to be mounted to the mgmd containers
func (mss *mgmdStatefulSet) getVolumeMounts() []v1.VolumeMount {
	return []v1.VolumeMount{
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
func (mss *mgmdStatefulSet) getInitContainers(nc *v1alpha1.NdbCluster) []v1.Container {
	// Command and args to run the mgmd init script
	cmdAndArgs := []string{
		helperScriptsMountPath + "/" + waitForDNSUpdateScriptKey,
	}

	return mss.createContainers(nc, cmdAndArgs, mss.getVolumeMounts(), nil, nil, true)
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

	startupProbe := &v1.Probe{
		Handler: v1.Handler{
			Exec: &v1.ExecAction{
				Command: []string{
					"/bin/bash",
					helperScriptsMountPath + "/" + mgmdStartupProbeKey,
				},
			},
		},
		// Startup probe - expects mgmd to get ready within a minute
		PeriodSeconds:    1,
		TimeoutSeconds:   3,
		FailureThreshold: 60,
	}

	// Readiness probe
	readinessProbe := &v1.Probe{
		Handler: portCheckHandler,
	}

	return mss.createContainers(nc, cmdAndArgs, volumeMounts, startupProbe, readinessProbe, false)
}

func (mss *mgmdStatefulSet) getPodAntiAffinity() *v1.PodAntiAffinity {
	// Default pod AntiAffinity rules for Management Nodes
	return getPodAntiAffinityRules([]string{
		mysqldClientName, sfsetTypeNdbd, sfsetTypeMgmd,
	})
}

// NewStatefulSet returns the StatefulSet specification to start and manage the Management nodes.
func (mss *mgmdStatefulSet) NewStatefulSet(cs *ndbconfig.ConfigSummary, nc *v1alpha1.NdbCluster) *apps.StatefulSet {
	statefulSet := mss.newStatefulSet(nc, cs)
	statefulSetSpec := &statefulSet.Spec

	// Fill in mgmd specific values
	replicas := cs.NumOfManagementNodes
	statefulSetSpec.Replicas = &replicas
	// Set pod management policy to start Management nodes one by one
	statefulSetSpec.PodManagementPolicy = apps.OrderedReadyPodManagement

	// Update template pod spec
	podSpec := &statefulSetSpec.Template.Spec
	podSpec.InitContainers = mss.getInitContainers(nc)
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
			Name: helperScriptsVolName,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: nc.GetConfigMapName(),
					},
					DefaultMode: &ownerCanExecMode,
					Items: []v1.KeyToPath{
						{
							Key:  dataNodeStartupProbeKey,
							Path: dataNodeStartupProbeKey,
						},
						{
							Key:  dataNodeInitScriptKey,
							Path: dataNodeInitScriptKey,
						},
						{
							// Load the wait-for-dns-update script.
							// It will be used by the init container.
							Key:  waitForDNSUpdateScriptKey,
							Path: waitForDNSUpdateScriptKey,
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
		// Volume mount for helper scripts
		nss.getHelperScriptVolumeMount(),
	}
}

// getInitContainers returns the init containers to be used by the data Node
func (nss *ndbdStatefulSet) getInitContainers(nc *v1alpha1.NdbCluster) []v1.Container {
	// Command and args to run the Data node init script
	cmdAndArgs := []string{
		helperScriptsMountPath + "/" + dataNodeInitScriptKey,
		nc.GetConnectstring(),
	}

	return nss.createContainers(nc, cmdAndArgs, nss.getVolumeMounts(nc), nil, nil, true)
}

// getContainers returns the containers to run a data Node
func (nss *ndbdStatefulSet) getContainers(nc *v1alpha1.NdbCluster) []v1.Container {

	// Command and args to run the Data node
	cmdAndArgs := []string{
		"/usr/sbin/ndbmtd",
		"-c", nc.GetConnectstring(),
		"--foreground",
		// Pass the nodeId to be used to prevent invalid
		// nodeId allocation during statefulset patching.
		"--ndb-nodeid=$(cat " + dataNodeIdFilePath + ")",
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
	startupProbe := &v1.Probe{
		Handler: v1.Handler{
			Exec: &v1.ExecAction{
				// datanode-startup-probe.sh
				Command: []string{
					"/bin/bash",
					helperScriptsMountPath + "/" + dataNodeStartupProbeKey,
				},
			},
		},
		// expect data node to get ready within 15 minutes
		PeriodSeconds:    2,
		TimeoutSeconds:   2,
		FailureThreshold: 450,
	}

	return nss.createContainers(nc, cmdAndArgs, volumeMounts, startupProbe, nil, false)
}

func (nss *ndbdStatefulSet) getPodAntiAffinity() *v1.PodAntiAffinity {
	// Default pod AntiAffinity rules for Data Nodes
	return getPodAntiAffinityRules([]string{
		sfsetTypeMgmd, mysqldClientName, sfsetTypeNdbd,
	})
}

// NewStatefulSet returns the StatefulSet specification to start and manage the Data nodes.
func (nss *ndbdStatefulSet) NewStatefulSet(cs *ndbconfig.ConfigSummary, nc *v1alpha1.NdbCluster) *apps.StatefulSet {
	statefulSet := nss.newStatefulSet(nc, cs)
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
	podSpec.InitContainers = nss.getInitContainers(nc)
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
