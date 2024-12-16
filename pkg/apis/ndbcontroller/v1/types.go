// Copyright (c) 2020, 2024, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package v1

import (
	"fmt"
	"strings"

	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/ndbconfig/configparser"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=ndb;ndbc,categories=all
//
// Additional printer columns
// +kubebuilder:printcolumn:name="Replica",type=string,JSONPath=`.spec.redundancyLevel`,description="Replica of the MySQL Cluster"
// +kubebuilder:printcolumn:name="Management Nodes",type=string,JSONPath=`.status.readyManagementNodes`,description="Number of ready MySQL Cluster Management Nodes"
// +kubebuilder:printcolumn:name="Data Nodes",type=string,JSONPath=`.status.readyDataNodes`,description="Number of ready MySQL Cluster Data Nodes"
// +kubebuilder:printcolumn:name="MySQL Servers",type=string,JSONPath=`.status.readyMySQLServers`,description="Number of ready MySQL Servers"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Age of the NdbCluster resource"
// +kubebuilder:printcolumn:name="Up-To-Date",type="string",JSONPath=".status.conditions[?(@.type=='UpToDate')].status",description="Indicates if the MySQL Cluster configuration is up-to-date with the spec specified in the NdbCluster resource"

// NdbCluster is the Schema for the Ndb CRD API
type NdbCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// The desired state of a MySQL NDB Cluster.
	Spec NdbClusterSpec `json:"spec"`
	// The status of the NdbCluster resource and the MySQL Cluster managed by it.
	Status NdbClusterStatus `json:"status,omitempty"`
}

// NdbClusterPodSpec contains a subset of PodSpec fields which when set
// will be copied into to the podSpec of respective MySQL Cluster
// node workload definitions.
type NdbClusterPodSpec struct {
	// Total compute Resources required by this pod.
	// Cannot be updated.
	//
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	//
	// More info: https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#nodeselector
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// If specified, the pod's scheduling constraints
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// If specified, the pod will be dispatched by specified scheduler.
	// If not specified, the pod will be dispatched by default scheduler.
	// +optional
	SchedulerName string `json:"schedulerName,omitempty"`
	// If specified, the pod's tolerations.
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

// NdbManagementNodeSpec is the specification of management node in MySQL Cluster
type NdbManagementNodeSpec struct {
	// Config is a map of default MySQL Cluster Management node configurations.
	//
	// More info :
	// https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-params-mgmd.html
	// +optional
	Config map[string]*intstr.IntOrString `json:"config,omitempty"`
	// NdbPodSpec contains a subset of PodSpec fields which when
	// set will be copied into to the podSpec of Management node's
	// statefulset definition.
	// +optional
	NdbPodSpec *NdbClusterPodSpec `json:"ndbPodSpec,omitempty"`
	// EnableLoadBalancer exposes the management servers externally using the
	// kubernetes cloud provider's load balancer. By default, the operator creates a ClusterIP
	// type service to expose the management server pods internally within the kubernetes cluster.
	// If EnableLoadBalancer is set to true, a LoadBalancer type service will be created instead,
	// exposing the management Servers outside the kubernetes cluster.
	// +kubebuilder:default=false
	// +optional
	EnableLoadBalancer bool `json:"enableLoadBalancer,omitempty"`
}

// NdbDataNodeSpec is the specification of data node in MySQL Cluster
type NdbDataNodeSpec struct {
	// Config is a map of default MySQL Cluster Data node configurations.
	//
	// More info :
	// https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-params-ndbd.html
	// +optional
	Config map[string]*intstr.IntOrString `json:"config,omitempty"`
	// NdbPodSpec contains a subset of PodSpec fields which when
	// set will be copied into to the podSpec of Data node's statefulset
	// definition.
	// +optional
	NdbPodSpec *NdbClusterPodSpec `json:"ndbPodSpec,omitempty"`
	// The total number of data nodes in MySQL Cluster.
	// The node count needs to be a multiple of the
	// redundancyLevel. A maximum of 144 data nodes are
	// allowed to run in a single MySQL Cluster.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=144
	NodeCount int32 `json:"nodeCount"`
	// PVCSpec is the PersistentVolumeClaimSpec to be used as the
	// VolumeClaimTemplate of the data node statefulset. A PVC will be created
	// for each data node by the statefulset controller and will be loaded into
	// the data node pod and the container.
	// +optional
	PVCSpec *corev1.PersistentVolumeClaimSpec `json:"pvcSpec,omitempty"`
}

// NdbMysqldSpec is the specification of MySQL Servers to be run as an SQL Frontend
type NdbMysqldSpec struct {
	// NodeCount is the number of MySQL Servers to be started by the Operator
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	NodeCount int32 `json:"nodeCount"`
	// MaxNodeCount is the count up to which the MySQL Servers would be
	// allowed to scale up without forcing a MySQL Cluster config update.
	// If unspecified, operator will define the MySQL Cluster config with
	// API sections for two additional MySQL Servers.
	// +optional
	MaxNodeCount int32 `json:"maxNodeCount,omitempty"`
	// ConnectionPoolSize is the number of connections a single
	// MySQL Server should use to connect to the MySQL Cluster nodes.
	// More info :
	// https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-options-variables.html#option_mysqld_ndb-cluster-connection-pool
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=63
	// +optional
	ConnectionPoolSize int32 `json:"connectionPoolSize,omitempty"`
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
	// EnableLoadBalancer exposes the MySQL servers externally using the kubernetes cloud
	// provider's load balancer. By default, the operator creates a ClusterIP type service
	// to expose the MySQL server pods internally within the kubernetes cluster. If
	// EnableLoadBalancer is set to true, a LoadBalancer type service will be created instead,
	// exposing the MySQL servers outside the kubernetes cluster.
	// +kubebuilder:default=false
	// +optional
	EnableLoadBalancer bool `json:"enableLoadBalancer,omitempty"`
	// NdbPodSpec contains a subset of K8s PodSpec fields which when set
	// will be copied into to the podSpec of MySQL Server StatefulSet.
	// +optional
	NdbPodSpec *NdbClusterPodSpec `json:"ndbPodSpec,omitempty"`
	// InitScripts is a map of configMap names from the same namespace and
	// optionally an array of keys which store the SQL scripts to be executed
	// during MySQL Server initialization. If key names are omitted, contents
	// of all the keys will be treated as initialization SQL scripts. All
	// scripts will be mounted into the MySQL pods and will be executed in the
	// alphabetical order of configMap names and key names.
	// +optional
	InitScripts map[string][]string `json:"initScripts,omitempty"`
	// PVCSpec is the PersistentVolumeClaimSpec to be used as the
	// VolumeClaimTemplate of the mysql server statefulset. A PVC will be created
	// for each mysql server by the statefulset controller and will be loaded into
	// the mysql server pod and the container.
	// +optional
	PVCSpec *corev1.PersistentVolumeClaimSpec `json:"pvcSpec,omitempty"`
}

// NdbClusterSpec defines the desired state of a MySQL NDB Cluster
type NdbClusterSpec struct {
	// The number of copies of all data stored in MySQL Cluster.
	// This also defines the number of nodes in a node group.
	// Supported values are 1, 2, 3, and 4.
	// Note that, setting this to 1 means that there is only a
	// single copy of all MySQL Cluster data and failure of any
	// Data node will cause the entire MySQL Cluster to fail.
	// The operator also implicitly decides the number of
	// Management nodes to be added to the MySQL Cluster
	// configuration based on this value. For a redundancy level
	// of 1, one Management node will be created. For 2 or
	// higher, two Management nodes will be created.
	// This value is immutable.
	//
	// More info :
	// https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-ndbd-definition.html#ndbparam-ndbd-noofreplicas
	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4
	// +optional
	RedundancyLevel int32 `json:"redundancyLevel,omitempty"`
	// ManagementNode specifies the configuration of the management node running in MySQL Cluster.
	// +optional
	ManagementNode *NdbManagementNodeSpec `json:"managementNode,omitempty"`
	// DataNode specifies the configuration of the data node running in MySQL Cluster.
	DataNode *NdbDataNodeSpec `json:"dataNode,omitempty"`
	// MysqlNode specifies the configuration of the MySQL Servers running in the cluster.
	// Note that the NDB Operator requires atleast one MySQL Server running in the cluster
	// for internal operations. If no MySQL Server is specified, the operator will by
	// default add one MySQL Server to the spec.
	// +optional
	MysqlNode *NdbMysqldSpec `json:"mysqlNode,omitempty"`
	// The number of extra API sections declared in the MySQL Cluster
	// config, in addition to the API sections declared implicitly
	// by the NDB Operator for the MySQL Servers.
	// Any NDBAPI application can connect to the MySQL Cluster via
	// these free slots.
	// +kubebuilder:default=2
	// +optional
	FreeAPISlots int32 `json:"freeAPISlots,omitempty"`
	// The name of the Secret that holds the encryption key or password
	// required for Transparent Data Encryption (TDE) in MySQL Cluster.
	// If a value is provided, the ndb operator will enable TDE and utilize the password
	// stored in the Secret as the file system password for all data nodes within the
	// MySQL Cluster. If no value is provided, TDE will not be enabled for MySQL Cluster.
	// +optional
	TDESecretName string `json:"tdeSecretName,omitempty"`
	// The name of the MySQL Ndb Cluster image to be used.
	// If not specified, "container-registry.oracle.com/mysql/community-cluster:9.2.0" will be used.
	// +kubebuilder:default="container-registry.oracle.com/mysql/community-cluster:9.2.0"
	// +optional
	Image string `json:"image,omitempty"`
	// ImagePullPolicy describes a policy for if/when to
	// pull the MySQL Cluster container image
	// +kubebuilder:validation:Enum:={Always, Never, IfNotPresent}
	// +kubebuilder:default:="IfNotPresent"
	// +optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// ImagePullSecretName optionally specifies the name of the secret that
	// holds the credentials required for pulling the MySQL Cluster image.
	// +optional
	ImagePullSecretName string `json:"imagePullSecretName,omitempty"`
}

// NdbClusterConditionType defines type for NdbCluster condition.
type NdbClusterConditionType string

const (
	// NdbClusterUpToDate specifies if the spec of the MySQL Cluster
	// is up-to-date with the NdbCluster resource spec
	NdbClusterUpToDate NdbClusterConditionType = "UpToDate"
)

const (
	// NdbClusterUptoDateReasonISR is the reason set when NdbClusterUpToDate
	// condition is set to False when the MySQL Cluster nodes are being
	// started for the first time.
	NdbClusterUptoDateReasonISR string = "InitialSystemRestart"
	// NdbClusterUptoDateReasonSpecUpdateInProgress is the reason used when
	// the NdbClusterUpToDate condition is set to False when a NdbCluster.Spec
	// change is being applied to the MySQL Cluster nodes.
	NdbClusterUptoDateReasonSpecUpdateInProgress string = "SpecUpdateInProgress"
	// NdbClusterUptoDateReasonSyncSuccess is the reason used when the
	// NdbClusterUpToDate condition is set to true once it is in sync
	// with the NdbCluster.Spec.
	NdbClusterUptoDateReasonSyncSuccess string = "SyncSuccess"
	// NdbClusterUptoDateReasonError is the reason used when
	// the NdbClusterUpToDate condition is set to False when sync
	// encounters an error.
	NdbClusterUptoDateReasonError string = "SyncError"
)

// NdbClusterCondition describes the state of a MySQL Cluster installation at a certain point.
type NdbClusterCondition struct {
	// Type of NdbCluster condition.
	Type NdbClusterConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human-readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// NdbClusterStatus is the status for a Ndb resource
type NdbClusterStatus struct {
	// ProcessedGeneration holds the latest generation of the
	// Ndb resource whose specs have been successfully applied
	// to the MySQL Cluster running inside K8s.
	ProcessedGeneration int64 `json:"processedGeneration,omitempty"`
	// The status of the MySQL Cluster Management nodes.
	ReadyManagementNodes string `json:"readyManagementNodes,omitempty"`
	// The status of the MySQL Cluster Data nodes.
	ReadyDataNodes string `json:"readyDataNodes,omitempty"`
	// The status of the MySQL Servers.
	ReadyMySQLServers string `json:"readyMySQLServers,omitempty"`
	// Conditions represent the latest available
	// observations of the MySQL Cluster's current state.
	Conditions []NdbClusterCondition `json:"conditions,omitempty"`
	// GeneratedRootPasswordSecretName is the name of the secret generated by the
	// operator to be used as the MySQL Server root account password. This will
	// be set to nil if a secret has been already provided to the operator via
	// spec.mysqlNode.rootPasswordSecretName.
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
		constants.ClusterLabel: nc.ObjectMeta.Name,
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
	return fmt.Sprintf("%s-%s", nc.ObjectMeta.Name, resource)
}

// GetServiceAccountName returns the Service account name
func (nc *NdbCluster) GetServiceAccountName() string {
	return fmt.Sprintf("%s-sa", nc.ObjectMeta.Name)
}

func (nc *NdbCluster) GetConfigMapName() string {
	return nc.ObjectMeta.Name + "-config"
}

// GetPodDisruptionBudgetName returns the PDB name of a given resource
func (nc *NdbCluster) GetPodDisruptionBudgetName(resource string) string {
	return fmt.Sprintf("%s-pdb-%s", nc.ObjectMeta.Name, resource)
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
	if nc.Spec.MysqlNode == nil {
		return 0
	}

	return nc.Spec.MysqlNode.NodeCount
}

// GetMySQLServerMaxNodeCount returns the MaxNodeCount value
func (nc *NdbCluster) GetMySQLServerMaxNodeCount() int32 {
	if nc.Spec.MysqlNode == nil {
		return 0
	}

	return nc.Spec.MysqlNode.MaxNodeCount
}

// GetMySQLServerConnectionPoolSize returns the connection pool size
func (nc *NdbCluster) GetMySQLServerConnectionPoolSize() int32 {
	if nc.Spec.MysqlNode == nil {
		return 0
	}

	return nc.Spec.MysqlNode.ConnectionPoolSize
}

// GetConnectstring returns the connect string of cluster represented by Ndb resource
func (nc *NdbCluster) GetConnectstring() string {
	port := "1186"

	connectstring := ""
	mgmdPodNamePrefix := nc.ObjectMeta.Name + "-mgmd"
	mgmdServiceName := nc.GetServiceName("mgmd")
	ndbNameSpace := nc.ObjectMeta.Namespace
	for i := int32(0); i < nc.GetManagementNodeCount(); i++ {
		if i > 0 {
			connectstring += ","
		}
		connectstring += fmt.Sprintf(
			"%s-%d.%s.%s.svc:%s", mgmdPodNamePrefix, i, mgmdServiceName, ndbNameSpace, port)
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
	if nc.Spec.MysqlNode == nil || nc.Spec.MysqlNode.MyCnf == "" {
		return ""
	}

	myCnf := nc.Spec.MysqlNode.MyCnf
	_, err := configparser.ParseString(myCnf)
	if err != nil && strings.Contains(err.Error(), "Non-empty line without section") {
		// section header is missing as it is optional - prepend and return
		myCnf = "[mysqld]\n" + myCnf
	}

	return myCnf
}

// GetWorkloadName returns the name K8s workload that manages the given NdbNodeType
func (nc *NdbCluster) GetWorkloadName(nodeType constants.NdbNodeType) string {
	return nc.ObjectMeta.Name + "-" + nodeType
}

// getCondition returns the NdbClusterCondition of condType from NdbCluster resource
func (nc *NdbCluster) getCondition(condType NdbClusterConditionType) *NdbClusterCondition {
	for _, condition := range nc.Status.Conditions {
		if condition.Type == condType {
			return &condition
		}
	}

	return nil
}

// HasSyncError returns if there is any error in the NdbClusterUpToDate condition
func (nc *NdbCluster) HasSyncError() bool {
	upToDateCond := nc.getCondition(NdbClusterUpToDate)
	return upToDateCond != nil && upToDateCond.Reason == NdbClusterUptoDateReasonError
}
