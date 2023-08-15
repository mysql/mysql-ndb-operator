// Copyright (c) 2020, 2023, Oracle and/or its affiliates.
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
	NdbNodeTypeAPI    NdbNodeType = "api"
)

const (
	// ClusterLabel is applied to all the resources owned by an
	// NdbCluster resource
	ClusterLabel = ndbcontroller.GroupName + "/v1"
	// ClusterNodeTypeLabel is applied to all the pods owned by an
	// NdbCluster resource
	ClusterNodeTypeLabel = ndbcontroller.GroupName + "/node-type"
	// ClusterResourceTypeLabel is applied to all K8s resources except
	// pods owned by an NdbCluster resource
	ClusterResourceTypeLabel = ndbcontroller.GroupName + "/resource-type"
)

const DataDir = "/var/lib/ndb"

const (
	// MaxNumberOfNodes is the maximum number of nodes in Ndb Cluster
	MaxNumberOfNodes = 256

	// MaxNumberOfDataNodes is the maximum number of nodes in Ndb Cluster
	MaxNumberOfDataNodes = 144

	// NdbOperatorDedicatedAPINodeId is the dedicated nodeId used by the
	// Ndb Operator to connect to a Management Server of a MySQL Cluster.
	// NodeIds 1-146 are reserved for 2 management nodes and
	// MaxNumberOfDataNodes count of data nodes.
	NdbOperatorDedicatedAPINodeId = 147

	// NdbNodeTypeAPIStartNodeId is the nodeId of the
	// first non-dedicated API/MySQLD section in MySQL Cluster config
	NdbNodeTypeAPIStartNodeId = NdbOperatorDedicatedAPINodeId + 1
)

// List of ConfigMap keys
const (
	// ConfigIniKey is the key to the management config string
	ConfigIniKey = "config.ini"
	// NumOfMySQLServers has the number of MySQL Servers declared in the NdbCluster spec.
	NumOfMySQLServers = "numOfMySQLServers"
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
	// TDEPasswordSecretName refers to the name of the secret that stores the password used for Transparent Data Encryption (TDE)
	TDEPasswordSecretName = "tdePasswordSecretName"
	// DataNodeInitialRestart indicates if the data nodes need to perform a initial restart
	DataNodeInitialRestart = "dataNodeInitialRestart"
)

// List of scripts loaded into the configmap
const (

	// MgmdStartupProbeScript is the Management Nodes' Startup Probe
	MgmdStartupProbeScript = "mgmd-startup-probe.sh"

	// DataNodeStartupProbeScript is the Data Nodes' Startup Probe
	DataNodeStartupProbeScript = "ndbmtd-startup-probe.sh"

	// MysqldInitScript is used to initialize the data directory of the MySQL Servers
	MysqldInitScript = "mysqld-init-script.sh"

	// MysqldHealthCheckScript is the script that checks MySQL Server's health
	MysqldHealthCheckScript = "mysqld-healthcheck.sh"
)
