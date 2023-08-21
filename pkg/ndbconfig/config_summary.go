// Copyright (c) 2020, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbconfig

import (
	"fmt"
	"strconv"

	"github.com/mysql/ndb-operator/config/debug"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
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
	// MySQLServerConfigVersion is the version of the my.cnf stored in the config map
	MySQLServerConfigVersion int32
	// NumOfManagementNodes is number of Management Nodes (1 or 2).
	NumOfManagementNodes int32
	// NumOfDataNodes is the number of Data Nodes.
	NumOfDataNodes int32
	// NumOfMySQLServers is the number of MySQL Servers
	// expected to connect to the MySQL Cluster data nodes.
	NumOfMySQLServers int32
	// NumOfMySQLServerSlots is the total number of [mysqld] sections required by the MySQL Servers.
	NumOfMySQLServerSlots int32
	// NumOfFreeApiSlots is the number of [api] sections declared in the config based on spec.freeApiSlots
	NumOfFreeApiSlots int32
	// RedundancyLevel is the number of replicas of the data stored in MySQL Cluster.
	RedundancyLevel int32
	// defaultNdbdSection has the values extracted from the default ndbd section of the management config.
	defaultNdbdSection configparser.Section
	// defaultMgmdSection has the values extracted from the default ndbd section of the management config.
	defaultMgmdSection configparser.Section
	// MySQLLoadBalancer indicates if the load balancer service for MySQL servers needs to be enabled
	MySQLLoadBalancer bool
	// ManagementLoadBalancer indicates if the load balancer service for management nodes needs to be enabled
	ManagementLoadBalancer bool
	// myCnfConfig has the parsed My.cnf config
	myCnfConfig configparser.ConfigIni
	// The host of the MySQL root user
	MySQLRootHost string
	// TDEPasswordSecretName refers to the name of the secret that stores the password used for Transparent Data Encryption (TDE)
	TDEPasswordSecretName string
	// DataNodeInitialRestart indicates if the data nodes need to perform a initial restart
	DataNodeInitialRestart bool
}

// parseInt32 parses the given string into an Int32
func parseInt32(strValue string) int32 {
	value, err := strconv.ParseInt(strValue, 10, 32)
	if err != nil {
		debug.Panic(fmt.Sprintf("parseUint32 failed to parse %s : %s", strValue, err.Error()))
	}
	return int32(value)
}

// parseBool parses the given string into an Bool
func parseBool(strValue string) bool {
	value, err := strconv.ParseBool(strValue)
	if err != nil {
		debug.Panic(fmt.Sprintf("parseBool failed to parse %s : %s", strValue, err.Error()))
	}
	return value
}

// NewConfigSummary creates a new ConfigSummary with the information extracted from the config map.
func NewConfigSummary(configMapData map[string]string) (*ConfigSummary, error) {

	config, err := configparser.ParseString(configMapData[constants.ConfigIniKey])
	if err != nil {
		// Should never happen as the operator generated the config.ini
		return nil, debug.InternalError(err)
	}

	cs := &ConfigSummary{
		NdbClusterGeneration: int64(parseInt32(configMapData[constants.NdbClusterGeneration])),
		MySQLClusterConfigVersion: parseInt32(
			config.GetValueFromSection("system", "ConfigGenerationNumber")),
		NumOfManagementNodes:  int32(config.GetNumberOfSections("ndb_mgmd")),
		NumOfDataNodes:        int32(config.GetNumberOfSections("ndbd")),
		NumOfMySQLServers:     parseInt32(configMapData[constants.NumOfMySQLServers]),
		NumOfMySQLServerSlots: int32(config.GetNumberOfSections("mysqld")),
		NumOfFreeApiSlots:     int32(config.GetNumberOfSections("api")),
		RedundancyLevel: parseInt32(
			config.GetValueFromSection("ndbd default", "NoOfReplicas")),
		MySQLLoadBalancer:      parseBool(configMapData[constants.MySQLLoadBalancer]),
		ManagementLoadBalancer: parseBool(configMapData[constants.ManagementLoadBalancer]),
		defaultNdbdSection:     config.GetSection("ndbd default"),
		defaultMgmdSection:     config.GetSection("mgmd default"),
		MySQLRootHost:          configMapData[constants.MySQLRootHost],
		TDEPasswordSecretName:  configMapData[constants.TDEPasswordSecretName],
		DataNodeInitialRestart: parseBool(configMapData[constants.DataNodeInitialRestart]),
	}

	// Update MySQL Config details if it exists
	mysqlConfigString := configMapData[constants.MySQLConfigKey]
	if mysqlConfigString != "" {
		cs.myCnfConfig, err = configparser.ParseString(mysqlConfigString)
		if err != nil {
			// Should never happen as the operator generated the my.cnf
			return nil, debug.InternalError(err)
		}
		cs.MySQLServerConfigVersion = parseInt32(
			cs.myCnfConfig.GetValueFromSection("header", "ConfigVersion"))
	}

	return cs, nil
}

// MySQLClusterConfigNeedsUpdate checks if the config of the MySQL Cluster needs to be updated.
func (cs *ConfigSummary) MySQLClusterConfigNeedsUpdate(nc *v1.NdbCluster) (needsUpdate bool) {

	newNdbdConfig := nc.Spec.DataNode.Config
	// Operator sets some default config parameters - take them into account when comparing configs.
	totalNdbdConfig := len(newNdbdConfig) + numOfOperatorSetConfigs
	// Add a count for EncryptedFileSystem if TDE is enabled
	if cs.TDEPasswordSecretName != "" {
		totalNdbdConfig = totalNdbdConfig + 1
	}
	// Check if the default ndbd section has been updated
	if totalNdbdConfig != len(cs.defaultNdbdSection) {
		// A config has been added (or) removed from default ndbd section
		return true
	}

	// Check if all configs exist and their value has not changed
	// TODO: Compare with actual values from the DataNodes
	for configKey, configValue := range newNdbdConfig {
		if value, exists := cs.defaultNdbdSection.GetValue(configKey); !exists || value != configValue.String() {
			// Either the config doesn't exist or the value has been changed
			return true
		}
	}

	// Check if the data nodes are being added
	if cs.NumOfDataNodes < nc.Spec.DataNode.NodeCount {
		return true
	}

	// Check if there is a change in the number of MySQL server
	// slots or number of free api slots.
	if cs.NumOfMySQLServerSlots != GetNumOfSectionsRequiredForMySQLServers(nc) {
		return true
	}

	// Check if there is a change in the TDE password
	if cs.TDEPasswordSecretName != GetTDESecretName(nc) {
		// If the field is unset in new or old config. Then the EncryptedFileSystem config parameter
		// needs to be added/deleted from the config.ini
		if cs.TDEPasswordSecretName == "" || GetTDESecretName(nc) == "" {
			return true
		}
	}

	// Check if more freeAPISlots slots are being declared by the new spec.
	// Note that the config will have an extra API slot dedicated for use by the NDB Operator
	if cs.NumOfFreeApiSlots != nc.Spec.FreeAPISlots+1 {
		// Number of free slots have been changed. Regenerate config and apply it.
		return true
	}

	// Check if the default mgmd section has been updated
	if nc.Spec.ManagementNode != nil {
		newMgmdConfig := nc.Spec.ManagementNode.Config
		if len(newMgmdConfig) != len(cs.defaultMgmdSection) {
			// A config has been added (or) removed from default mgmd section
			return true
		}
		// Check if all configs exist and their value has not changed
		for configKey, configValue := range newMgmdConfig {
			if value, exists := cs.defaultMgmdSection.GetValue(configKey); !exists || value != configValue.String() {
				// Either the config doesn't exist or the value has been changed
				return true
			}
		}
	}

	// No update required to the MySQL Cluster config.
	return false

}

// MySQLCnfNeedsUpdate checks if the my.cnf config stored in the configMap needs to be updated
func (cs *ConfigSummary) MySQLCnfNeedsUpdate(nc *v1.NdbCluster) (needsUpdate bool, err error) {
	myCnf := nc.GetMySQLCnf()

	if cs == nil {
		// NdbCluster resource created for the first time.
		// Need to update my.cnf if it is specified in the spec.
		return myCnf != "", nil
	}

	if myCnf == "" {
		// myCnf is empty.
		// Update required if it previously had a value
		return cs.myCnfConfig != nil, nil
	}

	myCnfConfig, err := configparser.ParseString(myCnf)
	if err != nil {
		// Cannot happen as it is already validated
		return false, debug.InternalError(err)
	}

	// Compare the configs and return if the config needs an update
	return !cs.myCnfConfig.IsEqual(myCnfConfig), nil
}
