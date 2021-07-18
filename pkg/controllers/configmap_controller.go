// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"encoding/json"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/resources"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"

	corev1 "k8s.io/api/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
)

type ConfigMapControlInterface interface {
	EnsureConfigMap(sc *SyncContext) (*corev1.ConfigMap, bool, error)
	PatchConfigMap(ndb *v1alpha1.NdbCluster, rc *resources.ResourceContext) (*corev1.ConfigMap, error)
	ExtractConfig(cm *corev1.ConfigMap) (string, error)
	DeleteConfigMap(ndb *v1alpha1.NdbCluster) error
}

type ConfigMapControl struct {
	ConfigMapControlInterface

	k8client kubernetes.Interface

	configMapLister       corelisters.ConfigMapLister
	configMapListerSynced cache.InformerSynced
}

// NewConfigMapControl creates a new ConfigMapControl
func NewConfigMapControl(client kubernetes.Interface,
	configMapInformer coreinformers.ConfigMapInformer) ConfigMapControlInterface {

	configMapControl := &ConfigMapControl{
		k8client:              client,
		configMapLister:       configMapInformer.Lister(),
		configMapListerSynced: configMapInformer.Informer().HasSynced,
	}

	return configMapControl
}

// ExtractConfig extracts the configuration file string from an existing config map
func (rcmc *ConfigMapControl) ExtractConfig(cm *corev1.ConfigMap) (string, error) {
	return resources.GetConfigFromConfigMapObject(cm)
}

// EnsureConfigMap creates a config map for the NdbCluster resource if one does not exist already
func (rcmc *ConfigMapControl) EnsureConfigMap(sc *SyncContext) (cm *corev1.ConfigMap, exists bool, err error) {

	ndb := sc.ndb
	configMapName := ndb.GetConfigMapName()
	configMapInterface := rcmc.k8client.CoreV1().ConfigMaps(ndb.Namespace)

	// Get the configmap with the name specified in Ndb.spec, fetching from client not cache
	cm, err = configMapInterface.Get(context.TODO(), configMapName, metav1.GetOptions{})

	if err == nil {
		// configmap already exists
		return cm, true, nil
	}

	if !errors.IsNotFound(err) {
		// failed to lookup the config map
		return nil, false, err
	}

	// configmap doesn't exist; create it.
	klog.Infof("Creating ConfigMap %s/%s", ndb.Namespace, configMapName)
	cm = resources.CreateConfigMap(ndb)
	cm, err = configMapInterface.Create(context.TODO(), cm, metav1.CreateOptions{})
	if err != nil {
		klog.Errorf("Failed to create config map %s/%s : %s", ndb.Namespace, configMapName, err)
	}

	// success
	return cm, false, nil
}

// PatchConfigMap patches the existing config map with new configuration data generated from ndb CRD object
func (rcmc *ConfigMapControl) PatchConfigMap(ndb *v1alpha1.NdbCluster, rc *resources.ResourceContext) (*corev1.ConfigMap, error) {

	// Get the StatefulSet with the name specified in Ndb.spec, fetching from client not cache
	cmOrg, err := rcmc.k8client.CoreV1().ConfigMaps(ndb.Namespace).Get(context.TODO(), ndb.GetConfigMapName(), metav1.GetOptions{})

	// If the resource doesn't exist
	if errors.IsNotFound(err) {
		klog.Errorf("Config map %s doesn't exist", ndb.GetConfigMapName())
		return nil, err
	}

	// Get an updated config map copy
	cmChg := resources.GetUpdatedConfigMap(ndb, cmOrg, rc)

	j, err := json.Marshal(cmOrg)
	if err != nil {
		return nil, err
	}

	j2, err := json.Marshal(cmChg)
	if err != nil {
		return nil, err
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(j, j2, corev1.ConfigMap{})
	if err != nil {
		return nil, err
	}

	var result *corev1.ConfigMap
	updateErr := wait.ExponentialBackoff(retry.DefaultBackoff, func() (ok bool, err error) {

		result, err = rcmc.k8client.CoreV1().ConfigMaps(ndb.Namespace).Patch(context.TODO(), cmOrg.Name,
			types.StrategicMergePatchType,
			patchBytes, metav1.PatchOptions{})

		if err != nil {
			klog.Errorf("Failed to patch config map: %v", err)
			return false, err
		}

		return true, nil
	})

	return result, updateErr
}
