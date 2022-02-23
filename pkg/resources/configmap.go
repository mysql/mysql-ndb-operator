// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"embed"
	"fmt"
	"strings"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/ndbconfig"
	"github.com/mysql/ndb-operator/pkg/ndbconfig/configparser"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

// Embed the helper scripts in the ndb operator binary
//go:embed scripts
var scriptsFS embed.FS

// updateManagementConfig updates the Data map with a new config.ini
// if there is any change to the MySQL Cluster configuration.
func updateManagementConfig(
	ndb *v1alpha1.NdbCluster, data map[string]string, oldConfigSummary *ndbconfig.ConfigSummary) error {

	// Update management if required
	if oldConfigSummary == nil || oldConfigSummary.MySQLClusterConfigNeedsUpdate(ndb) {
		// get the updated config string
		klog.Infof("MySQL Cluster config for NdbCluster resource %q needs to be updated", ndb.Name)
		configString, err := ndbconfig.GetConfigString(ndb, oldConfigSummary)
		if err != nil {
			klog.Errorf("Failed to get the config string : %v", err)
			return err
		}

		// add/update that to the data map
		data[constants.ConfigIniKey] = configString
	}

	// add/update the API slot information
	data[constants.FreeApiSlots] = fmt.Sprintf("%d", ndb.Spec.FreeAPISlots)
	data[constants.NumOfMySQLServers] = fmt.Sprintf("%d", ndb.GetMySQLServerNodeCount())

	return nil
}

// updateMySQLConfig updates the Data map with the
// default my.cnf if it is specified in the Ndb resource.
func updateMySQLConfig(ndb *v1alpha1.NdbCluster, data map[string]string) error {
	// Add the cnf, if any, to the data map
	myCnfValue := ndb.GetMySQLCnf()
	if len(myCnfValue) > 0 {
		_, err := configparser.ParseString(myCnfValue)
		if err != nil && strings.Contains(err.Error(), "Non-empty line without section") {
			// section header is missing as it is optional
			// add mysqld section header
			myCnfValue = "[mysqld]\n" + myCnfValue
		}
		data[mysqldMyCnfKey] = myCnfValue
	}
	return nil
}

// updateHelperScripts updates the data map with the helper
// scripts used for the MySQL Server initialisation & health
// probes and Data node health probe.
func updateHelperScripts(data map[string]string) error {
	// Extract and add the MySQL Server initializer file
	fileBytes, err := scriptsFS.ReadFile("scripts/" + mysqldInitScriptKey)
	if err != nil {
		klog.Errorf("Failed to read MySQL Server init script at %q : %v",
			mysqldInitScriptKey, err)
		return err
	}
	data[mysqldInitScriptKey] = string(fileBytes)

	// Extract and add the healthcheck script
	fileBytes, err = scriptsFS.ReadFile("scripts/" + mysqldHealthCheckKey)
	if err != nil {
		klog.Errorf("Failed to read MySQL Server healthcheck script at %q : %v",
			mysqldHealthCheckKey, err)
		return err
	}
	data[mysqldHealthCheckKey] = string(fileBytes)

	// Extract and add the data node healthcheck script
	fileBytes, err = scriptsFS.ReadFile("scripts/" + dataNodeHealthCheckKey)
	if err != nil {
		klog.Errorf("Failed to read Data node healthcheck script at %q : %v",
			dataNodeHealthCheckKey, err)
		return err
	}
	data[dataNodeHealthCheckKey] = string(fileBytes)
	return nil
}

// GetUpdatedConfigMap creates and returns a new config map with updated data
func GetUpdatedConfigMap(
	ndb *v1alpha1.NdbCluster, cm *corev1.ConfigMap, oldConfigSummary *ndbconfig.ConfigSummary) *corev1.ConfigMap {
	// create a deep copy of the original ConfigMap
	updatedCm := cm.DeepCopy()

	// Update the config.ini
	if err := updateManagementConfig(ndb, updatedCm.Data, oldConfigSummary); err != nil {
		klog.Errorf("Failed to update the config map : %v", err)
		return nil
	}

	// Update the MySQL custom config
	if err := updateMySQLConfig(ndb, updatedCm.Data); err != nil {
		klog.Errorf("Failed to update the config map : %v", err)
		return nil
	}

	// Update the generation the config map is based on
	updatedCm.Data[constants.NdbClusterGeneration] = fmt.Sprintf("%d", ndb.Generation)

	return updatedCm
}

// CreateConfigMap creates a config map object with the
// information available in the ndb object
func CreateConfigMap(ndb *v1alpha1.NdbCluster) *corev1.ConfigMap {

	/*
		kind: ConfigMap
		apiVersion: v1
		metadata:
			name: config-ini
			namespace: default
		data:
			config.ini: |
				[DB DEFAULT]
				....
	*/

	// Labels for the configmap
	cmLabels := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: "ndb-configmap",
	})

	// Data for the config map
	data := make(map[string]string)

	// Add the config ini value
	if updateManagementConfig(ndb, data, nil) != nil {
		return nil
	}

	// Add the MySQL custom config
	if updateMySQLConfig(ndb, data) != nil {
		return nil
	}

	// Add the helper scripts
	if updateHelperScripts(data) != nil {
		return nil
	}

	// Update the generation the config map is based on
	data[constants.NdbClusterGeneration] = fmt.Sprintf("%d", ndb.Generation)

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            ndb.GetConfigMapName(),
			Namespace:       ndb.Namespace,
			Labels:          cmLabels,
			OwnerReferences: ndb.GetOwnerReferences(),
		},
		Data: data,
	}
}
