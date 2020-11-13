// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package v1alpha1

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"io"

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
	DeploymentName string        `json:"deploymentname"`
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

	/* here we store the config hash of every
	   new generation of a spec that we received and thus acknowledged */
	ReceivedConfigHash string `json:"receivedConfigHash,omitempty"`
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

/* Calculate a hash of the current Spec */
/* TODO - not quite clear if its deterministic
there is no documented guarantee that reflect used in Marshal
has a guaranteed order of fields in the struct or if e.g. compiler could change it */
func (ndb *Ndb) calculateNewConfigHash() (string, error) {
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

	configHash, err := ndb.calculateNewConfigHash()
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
