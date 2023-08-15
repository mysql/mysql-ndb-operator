// Copyright (c) 2020, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"embed"
	"fmt"

	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/ndbconfig"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klog "k8s.io/klog/v2"
)

// Embed the helper scripts in the ndb operator binary
//
//go:embed statefulset/scripts
var scriptsFS embed.FS

// updateManagementConfig updates the Data map with a new config.ini
// if there is any change to the MySQL Cluster configuration.
func updateManagementConfig(
	ndb *v1.NdbCluster, data map[string]string, oldConfigSummary *ndbconfig.ConfigSummary) error {

	// Update management if required
	if oldConfigSummary == nil || oldConfigSummary.MySQLClusterConfigNeedsUpdate(ndb) {
		// get the updated config string
		if oldConfigSummary != nil {
			klog.Infof("MySQL Cluster config for NdbCluster resource %q needs to be updated", ndb.Name)
		}
		configString, err := ndbconfig.GetConfigString(ndb, oldConfigSummary)
		if err != nil {
			klog.Errorf("Failed to get the config string : %v", err)
			return err
		}

		// add/update that to the data map
		data[constants.ConfigIniKey] = configString
	}

	if oldConfigSummary != nil && oldConfigSummary.TDEPasswordSecretName != ndb.Spec.TDESecretName {
		// TDE password is changed, All data nodes need to perform initial node restart to adopt
		// to this change
		data[constants.DataNodeInitialRestart] = "true"
	}

	// add/update the API slot information
	data[constants.NumOfMySQLServers] = fmt.Sprintf("%d", ndb.GetMySQLServerNodeCount())

	// add/update service type info for management nodes
	data[constants.ManagementLoadBalancer] = fmt.Sprintf("%v",
		ndb.Spec.ManagementNode != nil && ndb.Spec.ManagementNode.EnableLoadBalancer)

	// add/update the TDE password secret name
	data[constants.TDEPasswordSecretName] = ndb.Spec.TDESecretName

	return nil
}

// updateMySQLConfig updates the my.cnf key in the configMap if required
func updateMySQLConfig(
	nc *v1.NdbCluster, data map[string]string, oldConfigSummary *ndbconfig.ConfigSummary) error {

	if needsUpdate, err := oldConfigSummary.MySQLCnfNeedsUpdate(nc); err != nil {
		klog.Errorf("Failed to check if the my.cnf needs to be updated : %s", err)
		return err
	} else if needsUpdate {
		// Update the my.cnf key in configmap
		if data[constants.MySQLConfigKey], err = ndbconfig.GetMySQLConfigString(nc, oldConfigSummary); err != nil {
			klog.Errorf("Failed to get the my.cnf config string : %s", err)
			return err
		}
	}

	// Add/update service type info and root host for MySQL servers
	if nc.Spec.MysqlNode != nil {
		data[constants.MySQLRootHost] = nc.Spec.MysqlNode.RootHost
		data[constants.MySQLLoadBalancer] = fmt.Sprintf("%v", nc.Spec.MysqlNode.EnableLoadBalancer)
	} else {
		data[constants.MySQLRootHost] = ""
		data[constants.MySQLLoadBalancer] = "false"
	}

	return nil
}

// updateHelperScripts updates the data map with the helper
// scripts used for the MySQL Server initialisation & health
// probes and Data node health probe.
func updateHelperScripts(data map[string]string) error {
	for fileName, desc := range map[string]string{
		constants.MysqldInitScript:           "MySQL Server init",
		constants.MysqldHealthCheckScript:    "MySQL Server Healthcheck",
		constants.DataNodeStartupProbeScript: "Data Node Startup Probe",
		constants.MgmdStartupProbeScript:     "Mgmd Startup Probe",
	} {
		fileBytes, err := scriptsFS.ReadFile("statefulset/scripts/" + fileName)
		if err != nil {
			klog.Errorf("Failed to read %s script at %q : %v",
				desc, fileName, err)
			return err
		}
		// Use the script file name as the key in configmap.
		data[fileName] = string(fileBytes)
	}
	return nil
}

// GetUpdatedConfigMap creates and returns a new config map with updated data
func GetUpdatedConfigMap(
	ndb *v1.NdbCluster, cm *corev1.ConfigMap, oldConfigSummary *ndbconfig.ConfigSummary) *corev1.ConfigMap {
	// create a deep copy of the original ConfigMap
	updatedCm := cm.DeepCopy()

	// If the generations are the same, the patch config map is only called to remove the
	// initial flag. When data node pods are restarted with the --initial flag, they lose all
	// data in their data directory and start fresh by rebuilding data from other nodes.
	// Therefore, using --initial in the data node container command is not appropriate. Once
	// the need for initial restart is over, the operator removes the --initial flag from the
	// data node container command. This ensures that when a pod goes down for any reason,
	// restarting the data node pod takes less time as the required data is already present
	// in the data node's directory.
	if oldConfigSummary != nil && oldConfigSummary.NdbClusterGeneration == ndb.Generation {
		updatedCm.Data[constants.DataNodeInitialRestart] = "false"
		return updatedCm
	}

	// Update the config.ini
	if err := updateManagementConfig(ndb, updatedCm.Data, oldConfigSummary); err != nil {
		klog.Errorf("Failed to update the config map : %v", err)
		return nil
	}

	// Update the MySQL custom config
	if err := updateMySQLConfig(ndb, updatedCm.Data, oldConfigSummary); err != nil {
		klog.Errorf("Failed to update the config map : %v", err)
		return nil
	}

	// Update the generation the config map is based on
	updatedCm.Data[constants.NdbClusterGeneration] = fmt.Sprintf("%d", ndb.Generation)

	return updatedCm
}

// CreateConfigMap creates a config map object with the
// information available in the ndb object
func CreateConfigMap(ndb *v1.NdbCluster) *corev1.ConfigMap {

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
	if updateMySQLConfig(ndb, data, nil) != nil {
		return nil
	}

	// Add the helper scripts
	if updateHelperScripts(data) != nil {
		return nil
	}

	// Add the data node initial restart value
	data[constants.DataNodeInitialRestart] = "false"

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
