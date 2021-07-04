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

	// ConfigHash used to create the config map with
	ConfigHash string
	// ConfigGeneration shows the generation the configuration is based on
	ConfigGeneration uint32
	// NodeGroupCount is the number of node groups in cluster configured in config
	ConfiguredNodeGroupCount uint32
	// ManagementNodeCount is the number of management nodes in cluster (1 or 2)
	ManagementNodeCount uint32
	// NumOfApiSlots is the number of API slots declared in the config
	NumOfApiSlots uint32
	// RedundancyLevel is the redundancy level configured
	RedundancyLevel uint32
}

// GetDataNodeCount returns the number of data nodes configured
func (rc *ResourceContext) GetDataNodeCount() uint32 {
	return rc.RedundancyLevel * rc.ConfiguredNodeGroupCount
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

	numOfDataNodes := config.GetNumberOfSections("ndbd")
	rc.ConfiguredNodeGroupCount = uint32(uint64(numOfDataNodes) / rl)

	rc.ManagementNodeCount = uint32(config.GetNumberOfSections("ndb_mgmd"))

	rc.NumOfApiSlots = uint32(config.GetNumberOfSections("mysqld"))

	return rc, nil
}
