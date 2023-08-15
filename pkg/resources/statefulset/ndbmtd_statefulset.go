// Copyright (c) 2022, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package statefulset

import (
	"strconv"

	"github.com/mysql/ndb-operator/config/debug"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/mgmapi"
	"github.com/mysql/ndb-operator/pkg/ndbconfig"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	klog "k8s.io/klog/v2"
)

var (
	// Ports to be exposed by the container and service
	ndbmtdPorts = []int32{1186}
)

// ndbmtdStatefulSet implements the NdbStatefulSetInterface to control a set of data nodes
type ndbmtdStatefulSet struct {
	baseStatefulSet
	secretLister listerscorev1.SecretLister
}

func (nss *ndbmtdStatefulSet) NewGoverningService(nc *v1.NdbCluster) *corev1.Service {
	return newService(nc, ndbmtdPorts, nss.nodeType, true, false)
}

// getPodVolumes returns a slice of volumes to be
// made available to the data node pods.
func (nss *ndbmtdStatefulSet) getPodVolumes(nc *v1.NdbCluster) []corev1.Volume {

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
					},
				},
			},
		},
	}

	// An empty directory volume needs to be provided
	// to the data node pods if the NdbCluster resource
	// doesn't have any PVCs defined to be used with
	// the data nodes.
	if nc.Spec.DataNode.PVCSpec == nil {
		podVolumes = append(podVolumes, *nss.getEmptyDirPodVolume(nss.getDataDirVolumeName()))
	}

	return podVolumes
}

// getVolumeMounts returns the volumes to be mounted to the ndbmtd containers
func (nss *ndbmtdStatefulSet) getVolumeMounts() []corev1.VolumeMount {
	// return volume mounts
	return []corev1.VolumeMount{
		{
			// Volume mount for data directory
			Name:      nss.getDataDirVolumeName(),
			MountPath: dataDirectoryMountPath,
		},
		// Volume mount for helper scripts
		nss.getHelperScriptVolumeMount(),
		// Mount the work dir volume
		nss.getWorkDirVolumeMount(),
	}
}

// getResourceRequestRequirements computes minimum memory required by the datanode
// from the MySQL Cluster config and returns the ResourceList with the calculated memory
func (nss *ndbmtdStatefulSet) getResourceRequestRequirements(nc *v1.NdbCluster) (corev1.ResourceList, error) {

	// Connect to the Management Server
	mgmClient, err := mgmapi.NewMgmClient(nc.GetConnectstring())
	if err != nil {
		klog.Errorf("Failed to connect to Management Server : %s", err)
		return nil, err
	}

	// Retrieve all the config values required to compute the memory requirements
	dataMemory, err := mgmClient.GetDataMemory(0)
	if err != nil {
		klog.Errorf("GetDataMemory failed with error %s", err)
		return nil, err
	}

	maxNoOfTables, err := mgmClient.GetMaxNoOfTables(0)
	if err != nil {
		klog.Errorf("GetMaxNoOfTables failed with error %s", err)
		return nil, err
	}

	maxNoOfAttributes, err := mgmClient.GetMaxNoOfAttributes(0)
	if err != nil {
		klog.Errorf("GetMaxNoOfAttributes failed with error %s", err)
		return nil, err
	}

	maxNoOfOrderedIndexes, err := mgmClient.GetMaxNoOfOrderedIndexes(0)
	if err != nil {
		klog.Errorf("GetMaxNoOfOrderedIndexes failed with error %s", err)
		return nil, err
	}

	maxNoOfUniqueHashIndexes, err := mgmClient.GetMaxNoOfUniqueHashIndexes(0)
	if err != nil {
		klog.Errorf("GetMaxNoOfUniqueHashIndexes failed with error %s", err)
		return nil, err
	}

	maxNoOfConcurrentOperations, err := mgmClient.GetMaxNoOfConcurrentOperations(0)
	if err != nil {
		klog.Errorf("GetMaxNoOfConcurrentOperations failed with error %s", err)
		return nil, err
	}

	transactionBufferMemory, err := mgmClient.GetTransactionBufferMemory(0)
	if err != nil {
		klog.Errorf("GetTransactionBufferMemory failed with error %s", err)
		return nil, err
	}

	indexMemory, err := mgmClient.GetIndexMemory(0)
	if err != nil {
		klog.Errorf("GetIndexMemory failed with error %s", err)
		return nil, err
	}

	redoBuffer, err := mgmClient.GetRedoBuffer(0)
	if err != nil {
		klog.Infof("GetRedoBuffer failed with error %s", err)
		return nil, err
	}

	longMessageBuffer, err := mgmClient.GetLongMessageBuffer(0)
	if err != nil {
		klog.Errorf("GetLongMessageBuffer failed with error %s", err)
		return nil, err
	}

	diskPageBufferMemory, err := mgmClient.GetDiskPageBufferMemory(0)
	if err != nil {
		klog.Errorf("GetDiskPageBufferMemory failed with error %s", err)
		return nil, err
	}

	sharedGlobalMemory, err := mgmClient.GetSharedGlobalMemory(0)
	if err != nil {
		klog.Errorf("GetSharedGlobalMemory failed with error %s", err)
		return nil, err
	}

	transactionMemory, err := mgmClient.GetTransactionMemory(0)
	if err != nil {
		klog.Errorf("GetTransactionMemory failed with error %s", err)
		return nil, err
	}

	noOfFragmentLogParts, err := mgmClient.GetNoOfFragmentLogParts(0)
	if err != nil {
		klog.Errorf("GetNoOfFragmentLogParts failed with error %s", err)
		return nil, err
	}

	//size of the ndbmtd executable inside the pod
	//mysql version: 8.0.29
	binarySize := uint64(12746520)

	if transactionMemory == 0 {
		transactionMemory = dataMemory / 10
	}

	totalMemory := dataMemory +
		uint64(maxNoOfTables) +
		uint64(maxNoOfAttributes) +
		uint64(maxNoOfOrderedIndexes) +
		uint64(maxNoOfUniqueHashIndexes) +
		uint64(maxNoOfConcurrentOperations) +
		uint64(transactionBufferMemory) +
		indexMemory +
		uint64(redoBuffer*noOfFragmentLogParts) + uint64(noOfFragmentLogParts) +
		uint64(longMessageBuffer) +
		diskPageBufferMemory +
		sharedGlobalMemory +
		transactionMemory +
		binarySize

	return corev1.ResourceList{
		"memory": resource.MustParse(strconv.FormatUint(totalMemory, 10)),
	}, nil
}

// getContainers returns the containers to run a data Node
func (nss *ndbmtdStatefulSet) getContainers(nc *v1.NdbCluster, addInitialFlag bool) ([]corev1.Container, error) {

	// Command and args to run the Data node
	cmdAndArgs := []string{
		"/usr/sbin/ndbmtd",
		"-c", nc.GetConnectstring(),
		"--foreground",
		// Pass the nodeId to be used to prevent invalid
		// nodeId allocation during statefulset patching.
		"--ndb-nodeid=$(cat " + NodeIdFilePath + ")",
	}

	if debug.Enabled {
		// Increase verbosity in debug mode
		cmdAndArgs = append(cmdAndArgs, "-v")
	}

	if nc.Spec.TDESecretName != "" {
		secret, err := nss.secretLister.Secrets(nc.Namespace).Get(nc.Spec.TDESecretName)
		if err != nil {
			// Secret does not exist
			klog.Errorf("Failed to retrieve Secret %q : %s", nc.Spec.TDESecretName, err)
			return nil, err
		}

		pass := string(secret.Data[corev1.BasicAuthPasswordKey])
		cmdAndArgs = append(cmdAndArgs, "--filesystem-password="+pass)
	}

	if addInitialFlag {
		cmdAndArgs = append(cmdAndArgs, "--initial")
	}

	ndbmtdContainer := nss.createContainer(
		nc, nss.getContainerName(false), cmdAndArgs,
		nss.getVolumeMounts(), ndbmtdPorts)

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
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				// ndbmtd-startup-probe.sh
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

	// Set resource request to data node container
	resList, err := nss.getResourceRequestRequirements(nc)
	if err == nil {
		ndbmtdContainer.Resources = corev1.ResourceRequirements{
			Requests: resList,
		}
	} else {
		klog.Warningf("Failed to set ResourceRequirements to %s", ndbmtdContainer.Name)
	}

	return []corev1.Container{ndbmtdContainer}, nil
}

func (nss *ndbmtdStatefulSet) getPodAntiAffinity() *corev1.PodAntiAffinity {
	// Default pod AntiAffinity rules for Data Nodes
	return GetPodAntiAffinityRules([]constants.NdbNodeType{
		constants.NdbNodeTypeMgmd, constants.NdbNodeTypeMySQLD, constants.NdbNodeTypeNdbmtd,
	})
}

// NewStatefulSet returns the StatefulSet specification to start and manage the Data nodes.
func (nss *ndbmtdStatefulSet) NewStatefulSet(cs *ndbconfig.ConfigSummary, nc *v1.NdbCluster) (*appsv1.StatefulSet, error) {
	var err error
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
	if nc.Spec.DataNode.PVCSpec != nil {
		statefulSetSpec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			// This PVC will be used as a template and an actual PVC will be created by the
			// statefulset controller with name "<data-dir-vol-name(i.e ndbmtd-data-vol)>-<pod-name>"
			*newPVC(nc, nss.getDataDirVolumeName(), nc.Spec.DataNode.PVCSpec),
		}
	}

	// Update template pod spec
	podSpec := &statefulSetSpec.Template.Spec
	podSpec.Containers, err = nss.getContainers(nc, cs.DataNodeInitialRestart)
	if err != nil {
		klog.Errorf("Failed to get containers for the statefulset %s", statefulSet.Name)
		return nil, err
	}
	podSpec.Volumes = append(podSpec.Volumes, nss.getPodVolumes(nc)...)
	// Set default AntiAffinity rules
	podSpec.Affinity = &corev1.Affinity{
		PodAntiAffinity: nss.getPodAntiAffinity(),
	}
	// Copy down any podSpec specified via CRD
	CopyPodSpecFromNdbPodSpec(podSpec, nc.Spec.DataNode.NdbPodSpec)

	return statefulSet, nil
}

// NewNdbmtdStatefulSet returns a new NdbStatefulSetInterface for data nodes
func NewNdbmtdStatefulSet(secretLister listerscorev1.SecretLister) NdbStatefulSetInterface {
	return &ndbmtdStatefulSet{
		baseStatefulSet{
			nodeType: constants.NdbNodeTypeNdbmtd,
		},
		secretLister,
	}
}
