// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package v1alpha1

import (
	"github.com/ocklin/ndb-operator/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
// Ndb is a specification for a Ndb resource
type Ndb struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NdbSpec   `json:"spec"`
	Status NdbStatus `json:"status"`
}

type NdbNdbdSpec struct {
	NoOfReplicas *int32 `json:"noofreplicas"`
	NodeCount    *int32 `json:"nodecount"`
	Name         string `json:"deploymentName"`
}

type NdbMgmdSpec struct {
	NodeCount *int32 `json:"nodecount"`
	Name      string `json:"name"`
}

type NdbMysqldSpec struct {
	NodeCount *int32 `json:"nodecount"`
	Name      string `json:"name"`
}

// NdbSpec is the spec for a Ndb resource
type NdbSpec struct {
	DeploymentName string        `json:"deploymentName"`
	Mgmd           NdbMgmdSpec   `json:"mgmd"`
	Ndbd           NdbNdbdSpec   `json:"ndbd"`
	Mysqld         NdbMysqldSpec `json:"mysqld"`

	// Config allows a user to specify a custom configuration file for MySQL.
	// +optional
	Config *corev1.LocalObjectReference `json:"config,omitempty"`
}

// NdbStatus is the status for a Ndb resource
type NdbStatus struct {
	ProcessedGeneration int64       `json:"processedGeneration,omitempty"`
	LastUpdate          metav1.Time `json:"lastUpdate,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NdbList is a list of Ndb resources
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
