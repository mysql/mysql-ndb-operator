// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"errors"
	"io/ioutil"
	"strings"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/config"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

const (
	configIniKey = "config.ini"
	MyCnfKey     = "my.cnf"
)

// GetConfigFromConfigMapObject returns the config string from the config map
func GetConfigFromConfigMapObject(cm *corev1.ConfigMap) (string, error) {

	if cm != nil && cm.Data != nil {
		if len(cm.Data) > 0 {
			if val, ok := cm.Data[configIniKey]; ok {
				return val, nil
			}
		}
	}

	return "", errors.New(configIniKey + " key not found in configmap")
}

// updateManagementConfig updates the Data map with latest config.ini
func updateManagementConfig(ndb *v1alpha1.Ndb, data map[string]string) error {
	// get the updated config string
	configString, err := GetConfigString(ndb)
	if err != nil {
		klog.Errorf("Failed to get the config string : %v", err)
		return err
	}

	// add/update that to the data map
	data[configIniKey] = configString
	return nil
}

// updateMySQLConfig updates the Data map with the
// configs/files required for the MySQL Server
func updateMySQLConfig(ndb *v1alpha1.Ndb, data map[string]string) error {
	// Put the MySQL Server initializer file as a config
	mysqlServerInitScriptPath := config.ScriptsDir + "/" + constants.NdbClusterInitScript

	fileBytes, err := ioutil.ReadFile(mysqlServerInitScriptPath)
	if err != nil {
		klog.Errorf("Failed to read MySQL Server init script at %s : %v",
			mysqlServerInitScriptPath, err)
		return err
	}

	// add it to the data map
	data[constants.NdbClusterInitScript] = string(fileBytes)

	// Add the cnf, if any, to the data map
	myCnfValue := ndb.GetMySQLCnf()
	if len(myCnfValue) > 0 {
		_, err = helpers.ParseString(myCnfValue)
		if err != nil && strings.Contains(err.Error(), "Non-empty line without section") {
			// section header is missing as it is optional
			// add mysqld section header
			myCnfValue = "[mysqld]\n" + myCnfValue
		}
		data[MyCnfKey] = myCnfValue
	}
	return nil
}

// GetUpdatedConfigMap creates and returns a new config map with updated data
func GetUpdatedConfigMap(ndb *v1alpha1.Ndb, cm *corev1.ConfigMap) *corev1.ConfigMap {
	// create a deep copy of the original ConfigMap
	updatedCm := cm.DeepCopy()

	// Update the config.ini
	if err := updateManagementConfig(ndb, updatedCm.Data); err != nil {
		klog.Errorf("Failed to update the config map : %v", err)
		return nil
	}

	// Update the mysqld config/cnf
	if err := updateMySQLConfig(ndb, updatedCm.Data); err != nil {
		klog.Errorf("Failed to update the config map : %v", err)
		return nil
	}

	return updatedCm
}

// CreateConfigMap creates a config map object with the
// information available in the ndb object
func CreateConfigMap(ndb *v1alpha1.Ndb) *corev1.ConfigMap {

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
	if updateManagementConfig(ndb, data) != nil {
		return nil
	}

	// Add the MySQL Configs
	if updateMySQLConfig(ndb, data) != nil {
		return nil
	}

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
