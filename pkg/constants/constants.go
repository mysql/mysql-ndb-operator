// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package constants

import "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller"

// NdbNodeType is the MySQL Cluster node type
type NdbNodeType = string

const (
	NdbNodeTypeMgmd   NdbNodeType = "mgmd"
	NdbNodeTypeNdbmtd NdbNodeType = "ndbmtd"
	NdbNodeTypeMySQLD NdbNodeType = "mysqld"
)

// ClusterLabel is applied to all components of a Ndb cluster
const ClusterLabel = ndbcontroller.GroupName + "/v1alpha1"
const ClusterNodeTypeLabel = ndbcontroller.GroupName + "/node-type"

// ClusterResourceTypeLabel is applied to all k8s resources managed by Ndb
const ClusterResourceTypeLabel = ndbcontroller.GroupName + "/resource-type"

const DataDir = "/var/lib/ndb"

// MaxNumberOfNodes is the maximum number of nodes in Ndb Cluster
const MaxNumberOfNodes = 256

// MaxNumberOfDataNodes is the maximum number of nodes in Ndb Cluster
const MaxNumberOfDataNodes = 144

// MaxNumberOfReplicas is the maximum number of replicas of Ndb (not K8)
const MaxNumberOfReplicas = 4

// List of ConfigMap keys
const (
	// ConfigIniKey is the key to the management config string
	ConfigIniKey = "config.ini"
	// NumOfMySQLServers has the number of MySQL Servers declared in the NdbCluster spec.
	NumOfMySQLServers = "numOfMySQLServers"
	// FreeApiSlots stores the number of freeApiSlots declared in the NdbCluster spec.
	FreeApiSlots = "freeApiSlots"
	// NdbClusterGeneration stores the generation the config map is based on.
	NdbClusterGeneration = "ndb.generation"
	// ManagementLoadBalancer indicates if the load balancer service for management nodes needs to be enabled
	ManagementLoadBalancer = "managementLoadBalancer"
	// MySQLLoadBalancer indicates if the load balancer service for MySQL servers needs to be enabled
	MySQLLoadBalancer = "mysqlLoadBalancer"
	// MySQLConfigKey is the key to the MySQL Server config(my.cnf) string
	MySQLConfigKey = "my.cnf"
	// MySQLRootHost is the key to the MySQL Server's root account's host.
	MySQLRootHost = "mysqlRootHost"
)

// List of scripts loaded into the configmap
const (
	// WaitForDNSUpdateScript waits for the current pod's DNS entry to get updated
	WaitForDNSUpdateScript = "wait-for-dns-update.sh"

	// MgmdStartupProbeScript is the Management Nodes' Startup Probe
	MgmdStartupProbeScript = "mgmd-startup-probe.sh"

	// DataNodeInitScript is used by the init containers of the data nodes
	DataNodeInitScript = "datanode-init-script.sh"

	// DataNodeStartupProbeScript is the Data Nodes' Startup Probe
	DataNodeStartupProbeScript = "datanode-startup-probe.sh"
)
