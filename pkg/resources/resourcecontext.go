// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"github.com/mysql/ndb-operator/pkg/helpers"
	"strconv"
)

// ResourceContext contains a summary of information extracted from the
// Management server's configuration. It is used during creation and
// updation of various K8s resources and also to compare any new incoming
// Ndb spec change
type ResourceContext struct {
	// ConfigHash is the hash of the Spec the config is based on.
	ConfigHash string
	// ConfigGeneration is the generation of the NdbCluster this configuration is based on.
	ConfigGeneration uint32
	// NumOfManagementNodes is number of Management Nodes (1 or 2).
	NumOfManagementNodes uint32
	// NumOfDataNodes is the number of Data Nodes.
	NumOfDataNodes uint32
	// NumOfMySQLServers is the number of MySQL Servers
	// expected to connect to the MySQL Cluster data nodes.
	NumOfMySQLServers uint32
	// NumOfApiSlots is the number of sections defined as [api].
	NumOfApiSlots uint32
	// RedundancyLevel is the number of replicas of the data stored in MySQL Cluster.
	RedundancyLevel uint32
}

// NewResourceContextFromConfiguration creates a new ResourceContext
// with extracted information from the configStr.
func NewResourceContextFromConfiguration(configStr string) (*ResourceContext, error) {

	config, err := helpers.ParseString(configStr)
	if err != nil {
		return nil, err
	}

	rc := &ResourceContext{}

	rc.ConfigHash = config.GetValueFromSection("header", "ConfigHash")

	generationStr := config.GetValueFromSection("system", "ConfigGenerationNumber")
	gen, _ := strconv.ParseUint(generationStr, 10, 32)
	rc.ConfigGeneration = uint32(gen)

	redundancyLevelStr := config.GetValueFromSection("ndbd default", "NoOfReplicas")
	rl, _ := strconv.ParseUint(redundancyLevelStr, 10, 32)
	rc.RedundancyLevel = uint32(rl)

	rc.NumOfDataNodes = uint32(config.GetNumberOfSections("ndbd"))

	rc.NumOfManagementNodes = uint32(config.GetNumberOfSections("ndb_mgmd"))

	buffer := config.GetValueFromSection("header", "NumOfMySQLServers")
	numOfMySQLServers, _ := strconv.ParseUint(buffer, 10, 32)
	rc.NumOfMySQLServers = uint32(numOfMySQLServers)

	rc.NumOfApiSlots = uint32(config.GetNumberOfSections("api"))

	return rc, nil
}
