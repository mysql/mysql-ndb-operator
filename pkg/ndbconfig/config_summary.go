// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbconfig

import (
	"fmt"
	"strconv"

	"github.com/mysql/ndb-operator/config/debug"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/ndbconfig/configparser"
)

// ConfigSummary contains a summary of information extracted from the
// configMap data. It is used during creation and updation of various
// K8s resources and also to compare any new incoming Ndb spec change.
type ConfigSummary struct {
	// NdbClusterGeneration is the generation of the NdbCluster this config is based on.
	NdbClusterGeneration int64
	// MySQLClusterConfigVersion is the version of the config.ini stored in the config map
	MySQLClusterConfigVersion int32
	// NumOfManagementNodes is number of Management Nodes (1 or 2).
	NumOfManagementNodes int32
	// NumOfDataNodes is the number of Data Nodes.
	NumOfDataNodes int32
	// NumOfMySQLServers is the number of MySQL Servers
	// expected to connect to the MySQL Cluster data nodes.
	NumOfMySQLServers int32
	// NumOfFreeApiSlots is the number of [api] sections declared in the config based on spec.freeApiSlots
	NumOfFreeApiSlots int32
	// TotalNumOfApiSlots is the total number of sections declared as [api] in the config.
	TotalNumOfApiSlots int32
	// RedundancyLevel is the number of replicas of the data stored in MySQL Cluster.
	RedundancyLevel int32
	// defaultNdbdConfigs has the values extracted from the default ndbd section of the management config.
	defaultNdbdSection configparser.Section
}

// parseInt32 parses the given string into an Int32
func parseInt32(strValue string) int32 {
	value, err := strconv.ParseInt(strValue, 10, 32)
	if err != nil {
		debug.Panic(fmt.Sprintf("parseUint32 failed to parse %s : %s", strValue, err.Error()))
	}
	return int32(value)
}

// NewConfigSummary creates a new ConfigSummary with the information extracted from the config map.
func NewConfigSummary(configMapData map[string]string) (*ConfigSummary, error) {

	config, err := configparser.ParseString(configMapData[constants.ConfigIniKey])
	if err != nil {
		return nil, debug.InternalError(err)
	}

	cs := &ConfigSummary{
		NdbClusterGeneration: int64(parseInt32(configMapData[constants.NdbClusterGeneration])),
		MySQLClusterConfigVersion: parseInt32(
			config.GetValueFromSection("system", "ConfigGenerationNumber")),
		NumOfManagementNodes: int32(config.GetNumberOfSections("ndb_mgmd")),
		NumOfDataNodes:       int32(config.GetNumberOfSections("ndbd")),
		NumOfMySQLServers:    parseInt32(configMapData[constants.NumOfMySQLServers]),
		NumOfFreeApiSlots:    parseInt32(configMapData[constants.FreeApiSlots]),
		TotalNumOfApiSlots:   int32(config.GetNumberOfSections("api")),
		RedundancyLevel: parseInt32(
			config.GetValueFromSection("ndbd default", "NoOfReplicas")),
		defaultNdbdSection: config.GetSection("ndbd default"),
	}

	return cs, nil
}

// getDefaultNdbdConfigValue returns the value of the config from defaultNdbdSection
func (cs *ConfigSummary) getDefaultNdbdConfigValue(configName string) string {
	if cs.defaultNdbdSection == nil {
		panic("default ndbd section is nil")
	}
	return cs.defaultNdbdSection[configName]
}

// MySQLClusterConfigNeedsUpdate checks if the config of the MySQL Cluster needs to be updated.
func (cs *ConfigSummary) MySQLClusterConfigNeedsUpdate(nc *v1alpha1.NdbCluster) (needsUpdate bool) {
	// Compare DataMemory
	// TODO: Compare with actual DataMemory in the system
	if nc.Spec.DataMemory != cs.getDefaultNdbdConfigValue("DataMemory") {
		return true
	}

	// Check if the number of API sections declared in the config is
	// sufficient. Calculate the total number of slots required for
	// the MySQL Servers and the NDB Operator. Update the config if
	// the existing number of sections are not enough.
	// Note : A MySQL scale up request will not trigger a config
	// update if the new requirements for the API slots can be
	// satisfied by the existing FreeApiSlots.
	totalApiSlotsRequired := getNumOfRequiredAPISections(nc, cs)
	if totalApiSlotsRequired > cs.TotalNumOfApiSlots {
		// We need more API slots than the ones already declared in the config.
		return true
	}

	// Check if more freeAPISlots slots are being declared by the new spec.
	if nc.Spec.FreeAPISlots != cs.NumOfFreeApiSlots {
		// Number of free slots have been changed. Regenerate config and apply it.
		return true
	}

	// TODO: How to update DataNodePVCs?

	// No update required to the MySQL Cluster config.
	return false

}
