// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"strconv"
	"strings"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers"

	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
)

const (
	mysqldClientName             = "mysqld"
	mysqldVolumeName             = mysqldClientName + "-volume"
	mysqldRootPasswordSecretName = mysqldClientName + "-root-password"
	mysqldDataDir                = constants.DataDir + "/mysql"
)

// MySQLServerDeployment is a deployment of MySQL Servers running as clients to the NDB
type MySQLServerDeployment struct {
	name string
}

// GetName returns the name of the MySQLServerDeployment
func (msd *MySQLServerDeployment) GetName() string {
	return msd.name
}

// Generate labels for the MySQL Server pods
func (msd *MySQLServerDeployment) getLabels(ndb *v1alpha1.Ndb) map[string]string {
	l := map[string]string{
		constants.ClusterNodeTypeLabel: mysqldClientName,
	}
	return labels.Merge(l, ndb.GetLabels())
}

// GetRootPasswordSecretName returns the name of the root password secret
func (msd *MySQLServerDeployment) GetRootPasswordSecretName() string {
	return msd.name + "-" + mysqldRootPasswordSecretName
}

// NewMySQLRootPasswordSecret creates and returns a new root password secret
func (msd *MySQLServerDeployment) NewMySQLRootPasswordSecret(ndb *v1alpha1.Ndb) *v1.Secret {
	// Generate a random password of length 16
	rootPassword := helpers.GenerateRandomPassword(16)
	// build Secret and return
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          ndb.GetLabels(),
			Name:            msd.GetRootPasswordSecretName(),
			Namespace:       ndb.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{ndb.GetOwnerReference()},
		},
		Data: map[string][]byte{v1.BasicAuthPasswordKey: []byte(rootPassword)},
		Type: v1.SecretTypeBasicAuth,
	}
}

// getPodVolumes returns the volumes to be used by the pod
func (msd *MySQLServerDeployment) getPodVolumes() *[]v1.Volume {
	onlyOwnerCanReadMode := int32(400)
	return &[]v1.Volume{
		// Use a temporary empty directory volume for the pod
		{
			Name: mysqldVolumeName,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		},
		// Use the root password secret as a volume
		{
			Name: mysqldRootPasswordSecretName,
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName: msd.GetRootPasswordSecretName(),
					// Project the password to a file name "root-password"
					Items: []v1.KeyToPath{{
						Key:  v1.BasicAuthPasswordKey,
						Path: "root-password",
					}},
					DefaultMode: &onlyOwnerCanReadMode,
				},
			},
		},
	}
}

// getMysqlVolumeMounts returns pod volumes to be mounted into the container
func (msd *MySQLServerDeployment) getMysqlVolumeMounts() *[]v1.VolumeMount {
	return &[]v1.VolumeMount{
		// Mount the empty dir volume as data directory
		{
			Name:      mysqldVolumeName,
			MountPath: mysqldDataDir,
		},
		// Mount the secret volume at /etc/auth
		{
			Name:      mysqldRootPasswordSecretName,
			MountPath: "/etc/auth",
		},
	}
}

// createContainer creates the MySQL Server container to be run as a client
func (msd *MySQLServerDeployment) createContainer(ndb *v1alpha1.Ndb) v1.Container {

	// MySQL Server arguments to run with NDB Cluster
	// TODO: Make these arguments configurable via CRD
	args := []string{
		// Enable ndbcluster engine and set connect string
		"--ndbcluster",
		"--ndb-connectstring=" + ndb.GetConnectstring(),
		"--user=mysql",
		"--datadir=" + mysqldDataDir,
		// Disable binlogging as these MySQL Servers won't be acting as replication sources
		"--skip-log-bin",
		// Enable maximum verbosity for development debugging
		"--ndb-extra-logging=99",
		"--log-error-verbosity=3",
	}

	entryPointArgs := strings.Join(args, " ")

	cmd := "/entrypoint.sh " + entryPointArgs

	imageName := ndb.Spec.ContainerImage
	klog.Infof("Creating MySQL container from image %s", imageName)

	return v1.Container{
		Name:  mysqldClientName,
		Image: imageName,
		Ports: []v1.ContainerPort{
			{
				ContainerPort: 3306,
			},
		},
		VolumeMounts:    *msd.getMysqlVolumeMounts(),
		Command:         []string{"/bin/bash", "-ecx", cmd},
		ImagePullPolicy: v1.PullIfNotPresent,
	}
}

// NewDeployment creates a new MySQL Server Deployment for the given Cluster.
func (msd *MySQLServerDeployment) NewDeployment(ndb *v1alpha1.Ndb, rc *ResourceContext) *apps.Deployment {

	podSpec := v1.PodSpec{
		Containers:         []v1.Container{msd.createContainer(ndb)},
		Volumes:            *msd.getPodVolumes(),
		ServiceAccountName: "ndb-agent",
	}

	// The deployment name to be used
	deploymentName := ndb.Name + "-" + mysqldClientName

	mysqldSpec := ndb.Spec.Mysqld
	podLabels := msd.getLabels(ndb)

	// Define the deployment
	mysqldDeployment := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			// Annotate the deployment with this config generation
			Annotations: map[string]string{
				constants.LastAppliedConfigGeneration: strconv.FormatInt(rc.ConfigGeneration, 10),
			},
			// The deployment name, namespace and owner references
			Name:            deploymentName,
			Namespace:       ndb.Namespace,
			Labels:          ndb.GetLabels(),
			OwnerReferences: []metav1.OwnerReference{ndb.GetOwnerReference()},
		},
		Spec: apps.DeploymentSpec{
			// The desired spec of the deployment
			Replicas: mysqldSpec.NodeCount,
			Selector: &metav1.LabelSelector{
				MatchLabels: podLabels,
			},
			Template: v1.PodTemplateSpec{
				// The template to be used to create the pods
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ndb.Namespace,
					Labels:    podLabels,
				},
				Spec: podSpec,
			},
		},
	}

	return mysqldDeployment
}

// NewMySQLServerDeployment creates a new MySQLServerDeployment
func NewMySQLServerDeployment(ndb *v1alpha1.Ndb) *MySQLServerDeployment {
	return &MySQLServerDeployment{
		name: ndb.Name + "-" + mysqldClientName,
	}
}
