// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package v1alpha1

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"github.com/mysql/ndb-operator/pkg/constants"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Redundancy Level",type=string,JSONPath=`.spec.redundancyLevel`
// +kubebuilder:resource:shortName=ndb;ndbc,categories=all

// NdbCluster is the Schema for the Ndb CRD API
type NdbCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NdbClusterSpec   `json:"spec"`
	Status NdbClusterStatus `json:"status,omitempty"`
}

// NdbMysqldSpec is the specification of MySQL Servers to be run as an SQL Frontend
type NdbMysqldSpec struct {
	// NodeCount is the number of MySQL Servers running in MySQL Cluster
	NodeCount int32 `json:"nodeCount"`
	// The name of the Secret that holds the password to be set for the MySQL
	// root accounts. The Secret should have a 'password' key that holds the
	// password.
	// If unspecified, a Secret will be created by the operator with a generated
	// name of format "<ndb-resource-name>-mysqld-root-password"
	// +optional
	RootPasswordSecretName string `json:"rootPasswordSecretName,omitempty"`
	// RootHost is the host or hosts from which the root user
	// can connect to the MySQL Server. If unspecified, root user
	// will be able to connect from any host that can access the MySQL Server.
	// +kubebuilder:default="%"
	// +optional
	RootHost string `json:"rootHost,omitempty"`
	// Configuration options to pass to the MySQL Server when it is started.
	// +optional
	MyCnf string `json:"myCnf,omitempty"`
}

// NdbClusterSpec defines the desired state of MySQL Ndb Cluster
type NdbClusterSpec struct {
	// The number of data replicas or copies of data stored in Ndb Cluster.
	// Supported and allowed values are 1, 2, 3, and 4.
	// A redundancy level of 1 creates a sharded cluster providing
	// NO fault tolerance in case of node failure.
	// With a redundancy level of 2 or higher cluster will continue
	// serving client requests even in case of failures.
	// 2 is the normal and most common value and the default.
	// A redundancy level of 3 provides additional protection.
	// For a redundancy level of 1 one management server will be created.
	// For 2 or higher two management servers will be used.
	// Once a cluster has been created, this number can NOT be easily changed.
	// More info :
	// https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-ndbd-definition.html#ndbparam-ndbd-noofreplicas
	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4
	RedundancyLevel int32 `json:"redundancyLevel,omitempty"`
	// The total number of data nodes in cluster.
	// The node count needs to be a multiple of the redundancyLevel.
	// Currently the maximum is 144 data nodes.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=144
	NodeCount int32 `json:"nodeCount"`
	// DataMemory specifies the space available per data node
	// for storing in memory tables and indexes.
	// Allowed values 1M - 1T. More info :
	// https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-ndbd-definition.html#ndbparam-ndbd-datamemory
	// +kubebuilder:default="98M"
	// +kubebuilder:validation:Pattern="[0-9]+[MGT]"
	DataMemory string `json:"dataMemory,omitempty"`

	// The name of the MySQL Ndb Cluster image to be used.
	// If not specified, "mysql/mysql-cluster:latest" will be used.
	// Lowest supported version is 8.0.26.
	// +kubebuilder:default="mysql/mysql-cluster:latest"
	// +optional
	Image string `json:"image,omitempty"`
	// ImagePullPolicy describes a policy for if/when to
	// pull the MySQL Cluster container image
	// +kubebuilder:validation:Enum:={Always, Never, IfNotPresent}
	// +kubebuilder:default:="IfNotPresent"
	// +optional
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// ImagePullSecretName optionally specifies the name of the secret that
	// holds the credentials required for pulling the MySQL Cluster image.
	// +optional
	ImagePullSecretName string `json:"imagePullSecretName,omitempty"`

	// DataNodePVCSpec is the PersistentVolumeClaimSpec to be used as the
	// VolumeClaimTemplate of the data node statefulset. A PVC will be created
	// for each data node by the statefulset controller and will be loaded into
	// the data node pod and the container.
	// +optional
	DataNodePVCSpec *v1.PersistentVolumeClaimSpec `json:"dataNodePVCSpec,omitempty"`

	// +optional
	Mysqld *NdbMysqldSpec `json:"mysqld,omitempty"`
}

// NdbClusterStatus is the status for a Ndb resource
type NdbClusterStatus struct {
	// ProcessedGeneration holds the latest generation of the
	// Ndb resource whose specs have been successfully applied
	// to the MySQL Cluster running inside K8s.
	ProcessedGeneration int64 `json:"processedGeneration,omitempty"`
	// LastUpdate is the time when the ProcessedGeneration was last updated.
	LastUpdate metav1.Time `json:"lastUpdate,omitempty"`
	// GeneratedRootPasswordSecretName is the name of the secret generated by the
	// operator to be used as the MySQL Server root account password. This will
	// be set to nil if a secret has been already provided to the operator via
	// spec.mysqld.rootPasswordSecretName.
	GeneratedRootPasswordSecretName string `json:"generatedRootPasswordSecretName,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NdbClusterList contains a list of Ndb resources
// +kubebuilder:object:root=true
type NdbClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []NdbCluster `json:"items"`
}

func (nc *NdbCluster) GetLabels() map[string]string {
	// Ndb main label ...
	l := map[string]string{
		constants.ClusterLabel: nc.Name,
	}
	return l
}

// GetCompleteLabels returns a complete list of labels by merging
// the given map of resourceLabels with the Ndb labels
func (nc *NdbCluster) GetCompleteLabels(resourceLabels map[string]string) map[string]string {
	return labels.Merge(nc.GetLabels(), resourceLabels)
}

// GetServiceName returns the Service name of a given resource
func (nc *NdbCluster) GetServiceName(resource string) string {
	return fmt.Sprintf("%s-%s", nc.Name, resource)
}

func (nc *NdbCluster) GetConfigMapName() string {
	return nc.Name + "-config"
}

// GetPodDisruptionBudgetName returns the PDB name of a given resource
func (nc *NdbCluster) GetPodDisruptionBudgetName(resource string) string {
	return fmt.Sprintf("%s-pdb-%s", nc.Name, resource)
}

// CalculateNewConfigHash Calculate a hash of the current Spec
/* TODO - not quite clear if its deterministic
there is no documented guarantee that reflect used in Marshal
has a guaranteed order of fields in the struct or if e.g. compiler could change it */
func (nc *NdbCluster) CalculateNewConfigHash() (string, error) {
	jsonNdb, err := json.Marshal(nc.Spec)
	if err != nil {
		return "", err
	}
	hash := md5.New()
	_, err = io.WriteString(hash, string(jsonNdb))
	if err != nil {
		return "", err
	}
	h := hash.Sum(nil)

	return base64.StdEncoding.EncodeToString(h), nil
}

// GetManagementNodeCount returns the number of
// management servers based on the redundancy levels
func (nc *NdbCluster) GetManagementNodeCount() int32 {
	if nc.Spec.RedundancyLevel == 1 {
		return 1
	}
	return 2
}

// GetMySQLServerNodeCount returns the number MySQL Servers
// connected to the NDB Cluster as an SQL frontend
func (nc *NdbCluster) GetMySQLServerNodeCount() int32 {
	if nc.Spec.Mysqld == nil {
		return 0
	}

	return nc.Spec.Mysqld.NodeCount
}

// GetConnectstring returns the connect string of cluster represented by Ndb resource
func (nc *NdbCluster) GetConnectstring() string {
	dnsZone := fmt.Sprintf("%s.svc.cluster.local", nc.Namespace)
	port := "1186"

	connectstring := ""
	mgmdPodNamePrefix := nc.Name + "-mgmd"
	mgmdServiceName := nc.GetServiceName("mgmd")
	for i := int32(0); i < nc.GetManagementNodeCount(); i++ {
		if i > 0 {
			connectstring += ","
		}
		connectstring += fmt.Sprintf(
			"%s-%d.%s.%s:%s", mgmdPodNamePrefix, i, mgmdServiceName, dnsZone, port)
	}

	return connectstring
}

// GetOwnerReferences returns a slice of OwnerReferences to
// be set to the resources owned by Ndb resource
func (nc *NdbCluster) GetOwnerReferences() []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(nc,
			schema.GroupVersionKind{
				Group:   SchemeGroupVersion.Group,
				Version: SchemeGroupVersion.Version,
				Kind:    "NdbCluster",
			})}
}

// GetMySQLCnf returns any specified additional MySQL Server cnf
func (nc *NdbCluster) GetMySQLCnf() string {
	if nc.Spec.Mysqld == nil {
		return ""
	}

	return nc.Spec.Mysqld.MyCnf
}
