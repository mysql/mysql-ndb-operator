// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package v1alpha1

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"io"

	"github.com/mysql/ndb-operator/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

// Specification of MySQL Servers to be run as an SQL Frontend
type NdbMysqldSpec struct {
	// NodeCount is the number of MySQL Servers running in MySQL Cluster
	NodeCount *int32 `json:"nodecount"`
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
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4
	RedundancyLevel *int32 `json:"redundancyLevel"`
	// The total number of data nodes in cluster.
	// The node count needs to be a multiple of the redundancyLevel.
	// Currently the maximum is 144 data nodes.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=144
	NodeCount *int32 `json:"nodecount"`
	// The name of the MySQL Ndb Cluster image to be used.
	// If not specified, "mysql-cluster:latest" will be used.
	// Lowest supported version is 8.0.22.
	// +kubebuilder:validation:Pattern="mysql-cluster:8.0.2[2-3]"
	// +optional
	ContainerImage string `json:"containerImage"`
	// +optional
	Mysqld NdbMysqldSpec `json:"mysqld"`
}

// NdbStatus is the status for a Ndb resource
type NdbStatus struct {
	ProcessedGeneration int64       `json:"processedGeneration,omitempty"`
	LastUpdate          metav1.Time `json:"lastUpdate,omitempty"`
	// The config hash of every new generation of a spec received and acknowledged
	ReceivedConfigHash string `json:"receivedConfigHash,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
// NdbList contains a list of Ndb resources
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

// Ndb data node label ...
func (ndb *Ndb) GetDataNodeLabels() map[string]string {
	l := map[string]string{
		constants.ClusterNodeTypeLabel: "ndbd",
	}
	return labels.Merge(l, ndb.GetLabels())
}

// Ndb management server label ...
func (ndb *Ndb) GetManagementNodeLabels() map[string]string {
	l := map[string]string{
		constants.ClusterNodeTypeLabel: "mgmd",
	}
	return labels.Merge(l, ndb.GetLabels())
}

//func (ndb *Ndb) GetServiceName() string {
//	return ndb.Name
//}

func (ndb *Ndb) GetManagementServiceName() string {
	return ndb.Name + "-mgmd"
}

func (ndb *Ndb) GetDataNodeServiceName() string {
	return ndb.Name + "-ndbd"
}

func (ndb *Ndb) GetConfigMapName() string {
	return ndb.Name + "-config"
}

func (ndb *Ndb) GetPodDisruptionBudgetName() string {
	return ndb.Name + "-pdb"
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

/* comparing the stored hash with the newly calculated hash of the Spec we see if it changed */
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

func (ndb *Ndb) GetRedundancyLevel() int {
	if ndb.Spec.RedundancyLevel == nil {
		return 2
	}
	return int(*ndb.Spec.RedundancyLevel)
}

func (ndb *Ndb) GetManagementNodeCount() int {
	if ndb.GetRedundancyLevel() == 1 {
		return 1
	}
	return 2
}
