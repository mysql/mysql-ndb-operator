// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package statefulset

import (
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller"
	"github.com/mysql/ndb-operator/pkg/constants"
)

const (
	// common data directory path for both mgmd and ndbmtd
	dataDirectoryMountPath = constants.DataDir + "/data"
	// dataNodeIdFilePath is the location of the file that has the data node's nodeId
	dataNodeIdFilePath = dataDirectoryMountPath + "/nodeId.val"

	// Common volume name and mount path for data node and mgmd node helper scripts
	helperScriptsVolName   = "helper-scripts-vol"
	helperScriptsMountPath = constants.DataDir + "/scripts"

	// LastAppliedConfigGeneration is the annotation key that holds the last applied config generation
	LastAppliedConfigGeneration = ndbcontroller.GroupName + "/last-applied-config-generation"
	// LastAppliedMySQLClusterConfigVersion is the annotation key that holds the last applied version of MySQL Cluster config
	LastAppliedMySQLClusterConfigVersion = ndbcontroller.GroupName + "/last-applied-mysql-cluster-config-version"
)

// Permissions to be set to the helper scripts loaded through configmap
var ownerCanExecMode = int32(0744)
