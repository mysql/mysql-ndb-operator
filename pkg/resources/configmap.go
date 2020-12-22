// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"errors"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

const (
	configIniKey = "config.ini"
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

// GetUpdatedConfigMap creates and returns a new config map with updated config.ini value
func GetUpdatedConfigMap(ndb *v1alpha1.Ndb, cm *corev1.ConfigMap) *corev1.ConfigMap {
	// create a deep copy of the original ConfigMap
	updatedCm := cm.DeepCopy()

	// Update the config.ini
	if err := updateManagementConfig(ndb, updatedCm.Data); err != nil {
		klog.Errorf("Failed to update the config map : %v", err)
		return nil
	}

	return updatedCm
}

func GenerateConfigMapObject(ndb *v1alpha1.Ndb) *corev1.ConfigMap {

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

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            ndb.GetConfigMapName(),
			Namespace:       ndb.Namespace,
			Labels:          cmLabels,
			OwnerReferences: []metav1.OwnerReference{ndb.GetOwnerReference()},
		},
		Data: data,
	}
}
