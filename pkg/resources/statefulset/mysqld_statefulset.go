// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package statefulset

import (
	"strconv"

	"github.com/mysql/ndb-operator/config/debug"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers"
	"github.com/mysql/ndb-operator/pkg/ndbconfig"
	"github.com/mysql/ndb-operator/pkg/resources"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
)

const (
	mysqldClientName = constants.NdbNodeTypeMySQLD
	// MySQL Server runtime directory
	mysqldDir = constants.DataDir

	// MySQL root password secret volume and mount path
	mysqldRootPasswordFileName  = ".root-password"
	mysqldRootPasswordVolName   = mysqldClientName + "-root-password-vol"
	mysqldRootPasswordMountPath = mysqldDir + "/auth"

	// MySQL Cluster init script volume and mount path
	mysqldInitScriptsVolName   = mysqldClientName + "-init-scripts-vol"
	mysqldInitScriptsMountPath = "/docker-entrypoint-initdb.d/"

	// my.cnf configmap key, volume and mount path
	mysqldCnfVolName   = mysqldClientName + "-cnf-vol"
	mysqldCnfMountPath = mysqldDir + "/cnf"

	// LastAppliedMySQLServerConfigVersion is the annotation key that holds the last applied version of MySQL Server config (my.cnf version)
	LastAppliedMySQLServerConfigVersion = ndbcontroller.GroupName + "/last-applied-my-cnf-config-version"
	// RootPasswordSecret is the name of the secret that holds the password for the root account
	RootPasswordSecret = ndbcontroller.GroupName + "/root-password-secret"
)

var (
	// Ports to be exposed by the container and service
	mysqldPorts = []int32{3306}
)

// mysqldStatefulSet implements the NdbStatefulSetInterface
// to control a set of MySQL Servers
type mysqldStatefulSet struct {
	baseStatefulSet
	configMapLister listerscorev1.ConfigMapLister
}

func (mss *mysqldStatefulSet) NewGoverningService(nc *v1alpha1.NdbCluster) *corev1.Service {
	return newService(nc, mysqldPorts, mss.nodeType, false, nc.Spec.Mysqld.EnableLoadBalancer)
}

// getPodVolumes returns the volumes to be used by the pod
func (mss *mysqldStatefulSet) getPodVolumes(ndb *v1alpha1.NdbCluster) ([]corev1.Volume, error) {
	allowOnlyOwnerToReadMode := int32(0400)
	rootPasswordSecretName, _ := resources.GetMySQLRootPasswordSecretName(ndb)
	podVolumes := []corev1.Volume{
		// Use the root password secret as a volume
		{
			Name: mysqldRootPasswordVolName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: rootPasswordSecretName,
					// Project the password to a file name "root-password"
					Items: []corev1.KeyToPath{
						{
							Key:  corev1.BasicAuthPasswordKey,
							Path: mysqldRootPasswordFileName,
						},
					},
					DefaultMode: &allowOnlyOwnerToReadMode,
				},
			},
		},
		// Load the healthcheck script via a volume
		{
			Name: helperScriptsVolName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: ndb.GetConfigMapName(),
					},
					Items: []corev1.KeyToPath{
						{
							Key:  constants.MysqldHealthCheckScript,
							Path: constants.MysqldHealthCheckScript,
						},
					},
				},
			},
		},
	}

	// Create a projected volume source to load all custom init scripts
	initScriptPvs := &corev1.ProjectedVolumeSource{
		Sources: []corev1.VolumeProjection{
			// Load the ndbcluster-init-script first
			{
				ConfigMap: &corev1.ConfigMapProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: ndb.GetConfigMapName(),
					},
					Items: []corev1.KeyToPath{
						{
							Key: constants.MysqldInitScript,
							// The docker entrypoint runs the init scripts in alphabetical order.
							// So, prefix the ndbcluster-init-script with 00 to ensure that it
							// gets run first before all the other custom SQL init scripts.
							Path: "00_" + constants.MysqldInitScript,
						},
					},
				},
			},
		},
	}

	// Create projections for all the scripts and append it to the initScriptPvs
	for configMapName, configMapKeys := range ndb.Spec.Mysqld.InitScripts {
		cm, err := mss.configMapLister.ConfigMaps(ndb.Namespace).Get(configMapName)
		if err != nil {
			klog.Errorf("Failed to get configMap '%s/%s' : %s", ndb.Namespace, configMapName, err)
			return nil, err
		}

		// Create a VolumeProjection with the configMap name
		volProjection := corev1.VolumeProjection{
			ConfigMap: &corev1.ConfigMapProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		}

		appendKeyToPathFromConfigMapKey := func(configMapName, key string, projection *corev1.ConfigMapProjection) {
			projection.Items = append(projection.Items, corev1.KeyToPath{
				Key: key,
				// Prefix the custom SQL scripts with 10 to ensure
				// that they get run after the ndbcluster-init-script.
				Path: "10_" + configMapName + "_" + key + ".sql",
			})
		}

		if len(configMapKeys) == 0 {
			// Keys not mentioned - extract all keys
			for key := range cm.Data {
				appendKeyToPathFromConfigMapKey(configMapName, key, volProjection.ConfigMap)
			}
		} else {
			// Keys from which the sql scripts have to be loaded are given.
			for _, key := range configMapKeys {
				appendKeyToPathFromConfigMapKey(configMapName, key, volProjection.ConfigMap)
			}
		}
		initScriptPvs.Sources = append(initScriptPvs.Sources, volProjection)
	}

	// Append the custom init script volume to the podVolumes
	podVolumes = append(podVolumes, corev1.Volume{
		Name: mysqldInitScriptsVolName,
		VolumeSource: corev1.VolumeSource{
			Projected: initScriptPvs,
		},
	})

	if len(ndb.GetMySQLCnf()) > 0 {
		// Load the cnf configmap key as a volume
		podVolumes = append(podVolumes, corev1.Volume{
			Name: mysqldCnfVolName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: ndb.GetConfigMapName(),
					},
					Items: []corev1.KeyToPath{
						{
							Key:  constants.MySQLConfigKey,
							Path: constants.MySQLConfigKey,
						},
					},
				},
			},
		})
	}

	// An empty directory volume needs to be provided to the mysql server
	// pods if the NdbCluster resource doesn't have any PVCs defined to
	// be used with the mysql servers.
	if ndb.Spec.Mysqld.PVCSpec == nil {
		podVolumes = append(podVolumes, *mss.getEmptyDirPodVolume(mss.getDataDirVolumeName()))
	}

	return podVolumes, nil
}

// getVolumeMounts returns pod volumes to be mounted into the container
func (mss *mysqldStatefulSet) getVolumeMounts(nc *v1alpha1.NdbCluster) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		// Mount the data directory volume
		{
			Name:      mss.getDataDirVolumeName(),
			MountPath: dataDirectoryMountPath,
		},
		// Mount the secret volume
		{
			Name:      mysqldRootPasswordVolName,
			MountPath: mysqldRootPasswordMountPath,
		},
		// Mount the init script volume
		{
			Name:      mysqldInitScriptsVolName,
			MountPath: mysqldInitScriptsMountPath,
		},
		// Volume mount for helper scripts
		mss.getHelperScriptVolumeMount(),
		// Mount the work dir volume
		mss.getWorkDirVolumeMount(),
	}

	if len(nc.GetMySQLCnf()) > 0 {
		// Mount the cnf volume
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      mysqldCnfVolName,
			MountPath: mysqldCnfMountPath,
		})
	}

	return volumeMounts
}

// getContainers returns the containers to run a MySQL Server
func (mss *mysqldStatefulSet) getContainers(nc *v1alpha1.NdbCluster) []corev1.Container {

	// Use the entrypoint included in the script to run MySQL Servers
	cmdAndArgs := []string{
		"/entrypoint.sh",
	}

	// Add the arguments to the command
	// first, pass any provided cnf options via defaults-file
	if len(nc.GetMySQLCnf()) > 0 {
		cmdAndArgs = append(cmdAndArgs,
			"--defaults-file="+mysqldCnfMountPath+"/"+constants.MySQLConfigKey)
	}

	// Add operator and NDB Cluster specific MySQL Server arguments
	cmdAndArgs = append(cmdAndArgs,
		// Enable ndbcluster engine and set connect string
		"--ndbcluster",
		"--ndb-connectstring="+nc.GetConnectstring(),
		"--user=mysql",
		"--datadir="+dataDirectoryMountPath,
		"--ndb-nodeid=$(cat "+NodeIdFilePath+")",
	)

	if debug.Enabled {
		cmdAndArgs = append(cmdAndArgs,
			// Enable maximum verbosity for development debugging
			"--ndb-extra-logging=99",
			"--log-error-verbosity=3",
		)
	}

	mysqldContainer := mss.createContainer(nc,
		mss.getContainerName(false),
		cmdAndArgs, mss.getVolumeMounts(nc), mysqldPorts)

	// Create an exec handler that runs the MysqldHealthCheckScript to be used in health probes
	healthProbeHandler := corev1.Handler{
		Exec: &corev1.ExecAction{
			Command: []string{
				"/bin/bash",
				helperScriptsMountPath + "/" + constants.MysqldHealthCheckScript,
			},
		},
	}

	// Setup health probes.
	// Startup probe - expects MySQL to get ready within 5 minutes
	mysqldContainer.StartupProbe = &corev1.Probe{
		Handler:          healthProbeHandler,
		PeriodSeconds:    2,
		FailureThreshold: 150,
	}

	// Readiness probe
	mysqldContainer.ReadinessProbe = &corev1.Probe{
		Handler: healthProbeHandler,
	}

	// Add Env variables required by MySQL Server
	ndbOperatorPodNamespace, _ := helpers.GetCurrentNamespace()
	mysqldContainer.Env = append(mysqldContainer.Env, corev1.EnvVar{
		// Path to the file that has the password of the root user
		// This will be consumed by the image entrypoint script
		Name:  "MYSQL_ROOT_PASSWORD",
		Value: mysqldRootPasswordMountPath + "/" + mysqldRootPasswordFileName,
	}, corev1.EnvVar{
		// Host from which the ndb operator user account can be accessed.
		// Use the hostname defined by the Ndb Operator deployment's template spec.
		Name:  "NDB_OPERATOR_ROOT_HOST",
		Value: "ndb-operator-pod.ndb-operator-svc." + ndbOperatorPodNamespace + ".svc.%",
	})

	return []corev1.Container{mysqldContainer}
}

func (mss *mysqldStatefulSet) getPodAntiAffinity() *corev1.PodAntiAffinity {
	// Default pod AntiAffinity rules for Data Nodes
	return GetPodAntiAffinityRules([]constants.NdbNodeType{
		constants.NdbNodeTypeMgmd, constants.NdbNodeTypeNdbmtd, constants.NdbNodeTypeMySQLD,
	})
}

// NewStatefulSet creates a new MySQL Server StatefulSet for the given NdbCluster.
func (mss *mysqldStatefulSet) NewStatefulSet(cs *ndbconfig.ConfigSummary, nc *v1alpha1.NdbCluster) (*appsv1.StatefulSet, error) {

	statefulSet := mss.newStatefulSet(nc, cs)
	statefulSetSpec := &statefulSet.Spec

	// Fill in MySQL Server specific details
	replicas := nc.GetMySQLServerNodeCount()
	statefulSetSpec.Replicas = &replicas
	// Set pod management policy to start MySQL Servers in parallel
	statefulSetSpec.PodManagementPolicy = appsv1.ParallelPodManagement

	// Update statefulset annotation
	statefulSetAnnotations := statefulSet.GetAnnotations()
	statefulSetAnnotations[RootPasswordSecret], _ = resources.GetMySQLRootPasswordSecretName(nc)

	// Add VolumeClaimTemplate if data node PVC Spec exists
	if nc.Spec.Mysqld.PVCSpec != nil {
		statefulSetSpec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{
			// This PVC will be used as a template and an actual PVC will be created by the
			// statefulset controller with name "<data-dir-vol-name(i.e mysqld-data-vol)>-<pod-name>"
			*newPVC(nc, mss.getDataDirVolumeName(), nc.Spec.Mysqld.PVCSpec),
		}
	}

	// Update template pod spec
	podSpec := &statefulSetSpec.Template.Spec
	podSpec.Containers = mss.getContainers(nc)

	podVolumes, err := mss.getPodVolumes(nc)
	if err != nil {
		klog.Errorf("Failed to get pod volumes for the statefulset %s", statefulSet.Name)
		return nil, err
	}
	podSpec.Volumes = append(podSpec.Volumes, podVolumes...)

	// Set default AntiAffinity rules
	podSpec.Affinity = &corev1.Affinity{
		PodAntiAffinity: mss.getPodAntiAffinity(),
	}
	// Copy down any podSpec specified via CRD
	CopyPodSpecFromNdbPodSpec(podSpec, nc.Spec.Mysqld.PodSpec)

	// Annotate the spec template with my.cnf version to trigger
	// an update of MySQL Servers when my.cnf changes.
	podAnnotations := statefulSetSpec.Template.GetAnnotations()
	podAnnotations[LastAppliedMySQLServerConfigVersion] = strconv.FormatInt(int64(cs.MySQLServerConfigVersion), 10)

	return statefulSet, nil
}

// NewMySQLdStatefulSet returns a new mysqldStatefulSet
func NewMySQLdStatefulSet(configMapLister listerscorev1.ConfigMapLister) NdbStatefulSetInterface {
	return &mysqldStatefulSet{
		baseStatefulSet{
			nodeType: constants.NdbNodeTypeMySQLD,
		},
		configMapLister,
	}
}
