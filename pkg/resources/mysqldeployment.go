// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"strconv"
	"strings"

	"github.com/mysql/ndb-operator/config/debug"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/ndbconfig"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

const (
	mysqldClientName = "mysqld"
	// MySQL Server runtime directory
	mysqldDir = constants.DataDir + "/mysqld"
	// Data directory volume and mount path
	mysqldDataDirVolName = mysqldClientName + "-vol"
	mysqldDataDir        = mysqldDir + "/datadir"

	// MySQL root password secret volume and mount path
	mysqldRootPasswordFileName  = ".root-password"
	mysqldRootPasswordVolName   = mysqldClientName + "-root-password-vol"
	mysqldRootPasswordMountPath = mysqldDir + "/auth"

	// MySQL Cluster init script name, volume and mount path
	mysqldInitScriptKey        = "ndbcluster-init-script.sh"
	mysqldInitScriptsVolName   = mysqldClientName + "-init-scripts-vol"
	mysqldInitScriptsMountPath = "/docker-entrypoint-initdb.d/"

	// my.cnf configmap key, volume and mount path
	mysqldMyCnfKey     = "my.cnf"
	mysqldCnfVolName   = mysqldClientName + "-cnf-vol"
	mysqldCnfMountPath = mysqldDir + "/cnf"

	// healthcheck.sh configmap key, volume and mount path
	mysqldHealthCheckKey       = "ndbcluster-healthcheck.sh"
	mysqldHealthCheckVolName   = mysqldClientName + "-healthcheck-vol"
	mysqldHealthCheckMountPath = mysqldDir + "/helper"

	// LastAppliedConfigGeneration is the annotation key that holds the last applied config generation
	LastAppliedConfigGeneration = ndbcontroller.GroupName + "/last-applied-config-generation"
	// RootPasswordSecret is the name of the secret that holds the password for the root account
	RootPasswordSecret = ndbcontroller.GroupName + "/root-password-secret"
)

// getContainerFromDeployment returns the container with the given name from the deployment
func getContainerFromDeployment(containerName string, deployment *apps.Deployment) *v1.Container {
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == containerName {
			return &container
		}
	}
	return nil
}

// MySQLServerDeployment is a deployment of MySQL Servers running as clients to the NDB
// TODO: Group all the methods inside a package rather than using this struct
type MySQLServerDeployment struct{}

// GetName returns the name of the MySQLServerDeployment
func (msd *MySQLServerDeployment) GetName(nc *v1alpha1.NdbCluster) string {
	return nc.Name + "-" + mysqldClientName
}

// GetTypeName returns the constants.ClusterNodeTypeLabel
// value of the resource that can be used as a pod selector.
func (msd *MySQLServerDeployment) GetTypeName() string {
	return mysqldClientName
}

// getDeploymentLabels returns the labels of the deployment
func (msd *MySQLServerDeployment) getDeploymentLabels(ndb *v1alpha1.NdbCluster) map[string]string {
	return ndb.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: mysqldClientName + "-deployment",
	})
}

// getPodLabels generates the labels for the MySQL Server pods controlled by the deployment
func (msd *MySQLServerDeployment) getPodLabels(ndb *v1alpha1.NdbCluster) map[string]string {
	return ndb.GetCompleteLabels(map[string]string{
		constants.ClusterNodeTypeLabel: mysqldClientName,
	})
}

// getPodVolumes returns the volumes to be used by the pod
func (msd *MySQLServerDeployment) getPodVolumes(ndb *v1alpha1.NdbCluster) []v1.Volume {
	allowOnlyOwnerToReadMode := int32(0400)
	rootPasswordSecretName, _ := GetMySQLRootPasswordSecretName(ndb)
	podVolumes := []v1.Volume{
		// Use a temporary empty directory volume for the pod
		{
			Name: mysqldDataDirVolName,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		},
		// Use the root password secret as a volume
		{
			Name: mysqldRootPasswordVolName,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: rootPasswordSecretName,
					// Project the password to a file name "root-password"
					Items: []v1.KeyToPath{{
						Key:  v1.BasicAuthPasswordKey,
						Path: mysqldRootPasswordFileName,
					}},
					DefaultMode: &allowOnlyOwnerToReadMode,
				},
			},
		},
		// Use the init script as a volume
		{
			Name: mysqldInitScriptsVolName,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: ndb.GetConfigMapName(),
					},
					// Load only the MySQL Server init scripts
					Items: []v1.KeyToPath{
						{
							Key:  mysqldInitScriptKey,
							Path: mysqldInitScriptKey,
						},
					},
				},
			},
		},
		// Load the healthcheck script as a volume
		{
			Name: mysqldHealthCheckVolName,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: ndb.GetConfigMapName(),
					},
					Items: []v1.KeyToPath{
						{
							Key:  mysqldHealthCheckKey,
							Path: mysqldHealthCheckKey,
						},
					},
				},
			},
		},
	}

	if len(ndb.GetMySQLCnf()) > 0 {
		// Load the cnf configmap key as a volume
		podVolumes = append(podVolumes, v1.Volume{
			Name: mysqldCnfVolName,
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: ndb.GetConfigMapName(),
					},
					Items: []v1.KeyToPath{
						{
							Key:  mysqldMyCnfKey,
							Path: mysqldMyCnfKey,
						},
					},
				},
			},
		})
	}

	return podVolumes
}

// getMysqlVolumeMounts returns pod volumes to be mounted into the container
func (msd *MySQLServerDeployment) getMysqlVolumeMounts(ndb *v1alpha1.NdbCluster) []v1.VolumeMount {
	volumeMounts := []v1.VolumeMount{
		// Mount the empty dir volume as data directory
		{
			Name:      mysqldDataDirVolName,
			MountPath: mysqldDataDir,
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
		// Mount the MySQL Server health script volume
		{
			Name:      mysqldHealthCheckVolName,
			MountPath: mysqldHealthCheckMountPath,
		},
	}

	if len(ndb.GetMySQLCnf()) > 0 {
		// Mount the cnf volume
		volumeMounts = append(volumeMounts, v1.VolumeMount{
			Name:      mysqldCnfVolName,
			MountPath: mysqldCnfMountPath,
		})
	}

	return volumeMounts
}

// createContainer creates the MySQL Server container to be run as a client
func (msd *MySQLServerDeployment) createContainer(ndb *v1alpha1.NdbCluster, oldContainer *v1.Container) *v1.Container {

	// Build the arguments to MySQL Server
	var args []string
	// first, pass any provided cnf options via defaults-file
	if len(ndb.GetMySQLCnf()) > 0 {
		args = append(args,
			"--defaults-file="+mysqldCnfMountPath+"/"+mysqldMyCnfKey)
	}

	// Add operator and NDB Cluster specific MySQL Server arguments
	args = append(args,
		// Enable ndbcluster engine and set connect string
		"--ndbcluster",
		"--ndb-connectstring="+ndb.GetConnectstring(),
		"--user=mysql",
		"--datadir="+mysqldDataDir,
		// Disable binlogging as these MySQL Servers won't be acting as replication sources
		"--skip-log-bin",
	)

	if debug.Enabled {
		args = append(args,
			// Enable maximum verbosity for development debugging
			"--ndb-extra-logging=99",
			"--log-error-verbosity=3",
		)
	}

	entryPointArgs := strings.Join(args, " ")

	cmd := "/entrypoint.sh " + entryPointArgs

	imageName := ndb.Spec.Image
	klog.Infof("Creating MySQL container from image %s", imageName)

	// exec handler to be used in health probes
	healthProbeHandler := v1.Handler{
		Exec: &v1.ExecAction{
			Command: []string{
				"/bin/bash",
				mysqldHealthCheckMountPath + "/" + mysqldHealthCheckKey,
			},
		},
	}

	container := &v1.Container{
		Name:  mysqldClientName,
		Image: imageName,
		Ports: []v1.ContainerPort{
			{
				ContainerPort: 3306,
			},
		},
		VolumeMounts:    msd.getMysqlVolumeMounts(ndb),
		Command:         []string{"/bin/bash", "-ecx", cmd},
		ImagePullPolicy: ndb.Spec.ImagePullPolicy,

		// Health probes.
		// Startup probe - expects MySQL to get ready within 5 minutes
		StartupProbe: &v1.Probe{
			Handler:          healthProbeHandler,
			PeriodSeconds:    2,
			FailureThreshold: 150,
		},
		// Readiness probe
		ReadinessProbe: &v1.Probe{
			Handler: healthProbeHandler,
		},
	}

	if oldContainer == nil {
		// Deployment being created for first time
		// Set the environment variables for the init scripts
		container.Env = []v1.EnvVar{
			{
				// Path to the file that has the password of the root user
				Name:  "MYSQL_ROOT_PASSWORD",
				Value: mysqldRootPasswordMountPath + "/" + mysqldRootPasswordFileName,
			},
			// MYSQL_CLUSTER_ROOT_HOST and MYSQL_CLUSTER_EXPECTED_REPLICAS
			// are consumed exactly once during the Deployment creation.
			// There are neither updated nor consumed during further deployment updates
			{
				// Host from which the root user can be accessed
				Name:  "MYSQL_CLUSTER_ROOT_HOST",
				Value: ndb.Spec.Mysqld.RootHost,
			},
			{
				// Expected replicas during initial setup
				Name:  "MYSQL_CLUSTER_EXPECTED_REPLICAS",
				Value: strconv.Itoa(int(ndb.GetMySQLServerNodeCount())),
			},
		}
	} else {
		// This is an Update to Deployment. Copy env variables from oldContainer
		// Although MYSQL_CLUSTER_ROOT_HOST and MYSQL_CLUSTER_EXPECTED_REPLICAS values
		// won't be consumed hereafter, we retain the values so as not to trigger a
		// template.spec update in the Deployments
		if oldContainer.Env != nil {
			in, out := &oldContainer.Env, &container.Env
			*out = make([]v1.EnvVar, len(*in))
			for i := range *in {
				(*in)[i].DeepCopyInto(&(*out)[i])
			}
		}
	}
	return container
}

// NewDeployment creates a new MySQL Server Deployment for the given Cluster.
func (msd *MySQLServerDeployment) NewDeployment(
	ndb *v1alpha1.NdbCluster, rc *ndbconfig.ResourceContext, oldDeployment *apps.Deployment) *apps.Deployment {

	var oldContainer *v1.Container
	if oldDeployment != nil {
		oldContainer = getContainerFromDeployment(mysqldClientName, oldDeployment)
	}

	podSpec := v1.PodSpec{
		Containers: []v1.Container{*msd.createContainer(ndb, oldContainer)},
		Volumes:    msd.getPodVolumes(ndb),
	}

	imagePullSecretName := ndb.Spec.ImagePullSecretName
	if imagePullSecretName != "" {
		podSpec.ImagePullSecrets = []v1.LocalObjectReference{
			{
				Name: imagePullSecretName,
			},
		}
	}

	// The deployment name to be used
	deploymentName := ndb.Name + "-" + mysqldClientName

	mysqlNodeCount := ndb.GetMySQLServerNodeCount()
	podLabels := msd.getPodLabels(ndb)

	// Root password secret
	rootPasswordSecret, _ := GetMySQLRootPasswordSecretName(ndb)

	// Define the deployment
	mysqldDeployment := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			// The deployment name, namespace and labels
			Name:      deploymentName,
			Namespace: ndb.Namespace,
			Labels:    msd.getDeploymentLabels(ndb),
			// Owner reference pointing to the Ndb resource
			OwnerReferences: ndb.GetOwnerReferences(),
			Annotations: map[string]string{
				RootPasswordSecret: rootPasswordSecret,
			},
		},
		Spec: apps.DeploymentSpec{
			// The desired spec of the deployment
			Replicas: &mysqlNodeCount,
			Selector: &metav1.LabelSelector{
				// must match templates labels
				MatchLabels: podLabels,
			},
			Template: v1.PodTemplateSpec{
				// The template to be used to create the pods
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ndb.Namespace,
					Labels:    podLabels,
					// Annotate the template with the current config generation
					// A change in the config will trigger a rolling update of the deployments
					// TODO: Trigger a rolling update only when there is a change in Ndb
					//       resource config that affects the MySQL Server
					Annotations: map[string]string{
						LastAppliedConfigGeneration: strconv.FormatUint(uint64(rc.ConfigGeneration), 10),
					},
				},
				Spec: podSpec,
			},
		},
	}

	return mysqldDeployment
}

// NewMySQLServerDeployment creates a new MySQLServerDeployment
func NewMySQLServerDeployment() *MySQLServerDeployment {
	return &MySQLServerDeployment{}
}
