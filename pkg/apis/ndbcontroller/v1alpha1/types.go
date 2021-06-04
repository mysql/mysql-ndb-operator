// Copyright (c) 2020, Oracle and/or its affiliates.
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

// Ndb is the Schema for the Ndb CRD API
type Ndb struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NdbSpec   `json:"spec"`
	Status NdbStatus `json:"status,omitempty"`
}

// NdbMysqldSpec is the specification of MySQL Servers to be run as an SQL Frontend
type NdbMysqldSpec struct {
	// NodeCount is the number of MySQL Servers running in MySQL Cluster
	NodeCount int32 `json:"nodecount"`
	// The name of the Secret that holds the password to be set for the MySQL
	// root accounts. The Secret should have a 'password' key that holds the
	// password.
	// If unspecified, a Secret will be created by the operator with a generated
	// name of format "<ndb-resource-name>-mysqld-root-password"
	// +optional
	RootPasswordSecretName string `json:"rootPasswordSecretName,omitempty"`
	// Configuration options to pass to the MySQL Server when it is started.
	// +optional
	MyCnf string `json:"myCnf,omitempty"`
}

// NdbSpec defines the desired state of MySQL Ndb Cluster
type NdbSpec struct {
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
	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4
	RedundancyLevel int32 `json:"redundancyLevel,omitempty"`
	// The total number of data nodes in cluster.
	// The node count needs to be a multiple of the redundancyLevel.
	// Currently the maximum is 144 data nodes.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=144
	NodeCount int32 `json:"nodecount"`
	// The name of the MySQL Ndb Cluster image to be used.
	// If not specified, "mysql/mysql-cluster:8.0.22" will be used.
	// Lowest supported version is 8.0.22.
	// +kubebuilder:default="mysql/mysql-cluster:8.0.22"
	// +kubebuilder:validation:Pattern="mysql/mysql-cluster:8.0.2[2-6](-[a-zA-Z0-9]*)?"
	// +optional
	ContainerImage string `json:"containerImage,omitempty"`

	// DataNodePVCSpec is the PersistentVolumeClaimSpec to be used as the
	// VolumeClaimTemplate of the data node statefulset. A PVC will be created
	// for each data node by the statefulset controller and will be loaded into
	// the data node pod and the container.
	// +optional
	DataNodePVCSpec *v1.PersistentVolumeClaimSpec `json:"dataNodePVCSpec,omitempty"`

	// +optional
	Mysqld *NdbMysqldSpec `json:"mysqld,omitempty"`
}

// NdbStatus is the status for a Ndb resource
type NdbStatus struct {
	// ProcessedGeneration holds the latest generation of the
	// Ndb resource whose specs have been successfully applied
	// to the MySQL Cluster running inside K8s.
	ProcessedGeneration int64 `json:"processedGeneration,omitempty"`
	// LastUpdate is the time when the ProcessedGeneration was last updated.
	LastUpdate metav1.Time `json:"lastUpdate,omitempty"`
	// The config hash of every new generation of a spec received and acknowledged
	ReceivedConfigHash string `json:"receivedConfigHash,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NdbList contains a list of Ndb resources
// +kubebuilder:object:root=true
type NdbList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Ndb `json:"items"`
}

func (ndb *Ndb) GetLabels() map[string]string {
	// Ndb main label ...
	l := map[string]string{
		constants.ClusterLabel: ndb.Name,
	}
	return l
}

// GetCompleteLabels returns a complete list of labels by merging
// the given map of resourceLabels with the Ndb labels
func (ndb *Ndb) GetCompleteLabels(resourceLabels map[string]string) map[string]string {
	return labels.Merge(ndb.GetLabels(), resourceLabels)
}

// GetServiceName returns the Service name of a given resource
func (ndb *Ndb) GetServiceName(resource string) string {
	return fmt.Sprintf("%s-%s", ndb.Name, resource)
}

func (ndb *Ndb) GetConfigMapName() string {
	return ndb.Name + "-config"
}

// GetPodDisruptionBudgetName returns the PDB name of a given resource
func (ndb *Ndb) GetPodDisruptionBudgetName(resource string) string {
	return fmt.Sprintf("%s-pdb-%s", ndb.Name, resource)
}

// CalculateNewConfigHash Calculate a hash of the current Spec
/* TODO - not quite clear if its deterministic
there is no documented guarantee that reflect used in Marshal
has a guaranteed order of fields in the struct or if e.g. compiler could change it */
func (ndb *Ndb) CalculateNewConfigHash() (string, error) {
	jsonNdb, err := json.Marshal(ndb.Spec)
	if err != nil {
		return "", err
	}
	hash := md5.New()
	io.WriteString(hash, string(jsonNdb))
	h := hash.Sum(nil)

	return base64.StdEncoding.EncodeToString(h), nil
}

// IsConfigHashEqual calculates the new hash of the Spec and
// compares it with the stored hash
func (ndb *Ndb) IsConfigHashEqual() (string, bool, error) {

	configHash, err := ndb.CalculateNewConfigHash()
	if err != nil {
		return "", false, err
	}
	if ndb.Status.ReceivedConfigHash == "" {
		return configHash, false, nil
	}
	if ndb.Status.ReceivedConfigHash != configHash {
		return configHash, false, nil
	}
	return configHash, true, nil
}

// GetManagementNodeCount returns the number of
// management servers based on the redundancy levels
func (ndb *Ndb) GetManagementNodeCount() int32 {
	if ndb.Spec.RedundancyLevel == 1 {
		return 1
	}
	return 2
}

// GetMySQLServerNodeCount returns the number MySQL Servers
// connected to the NDB Cluster as an SQL frontend
func (ndb *Ndb) GetMySQLServerNodeCount() int32 {
	if ndb.Spec.Mysqld == nil {
		return 0
	}

	return ndb.Spec.Mysqld.NodeCount
}

// GetConnectstring returns the connect string of cluster represented by Ndb resource
func (ndb *Ndb) GetConnectstring() string {
	dnsZone := fmt.Sprintf("%s.svc.cluster.local", ndb.Namespace)
	port := "1186"

	connectstring := ""
	mgmdPodNamePrefix := ndb.Name + "-mgmd"
	mgmdServiceName := ndb.GetServiceName("mgmd")
	for i := int32(0); i < ndb.GetManagementNodeCount(); i++ {
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
func (ndb *Ndb) GetOwnerReferences() []metav1.OwnerReference {
	return []metav1.OwnerReference{
		*metav1.NewControllerRef(ndb,
			schema.GroupVersionKind{
				Group:   SchemeGroupVersion.Group,
				Version: SchemeGroupVersion.Version,
				Kind:    "Ndb",
			})}
}

// GetMySQLCnf returns any specified additional MySQL Server cnf
func (ndb *Ndb) GetMySQLCnf() string {
	if ndb.Spec.Mysqld == nil {
		return ""
	}

	return ndb.Spec.Mysqld.MyCnf
}
