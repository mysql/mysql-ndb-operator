// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
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
	EnsureConfigMap(ndb *v1alpha1.Ndb) (*corev1.ConfigMap, bool, error)
	PatchConfigMap(ndb *v1alpha1.Ndb) (*corev1.ConfigMap, error)
	ExtractConfig(cm *corev1.ConfigMap) (string, error)
	DeleteConfigMap(ndb *v1alpha1.Ndb) error
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

// EnsureConfigMap creates a new config map if no config map for this ndb cluster exists
// it generations the configuration file and stores it in the config map
// EnsureConfigMap returns
//   the config map
//   true if it existed
//   an error if something went wrong
func (rcmc *ConfigMapControl) EnsureConfigMap(ndb *v1alpha1.Ndb) (*corev1.ConfigMap, bool, error) {

	// Get the StatefulSet with the name specified in Ndb.spec, fetching from client not cache
	cm, err := rcmc.k8client.CoreV1().ConfigMaps(ndb.Namespace).Get(ndb.GetConfigMapName(), metav1.GetOptions{})

	if err == nil {
		return cm, true, nil
	}

	if !errors.IsNotFound(err) {
		return nil, false, err
	}

	// If the resource doesn't exist, we'll create it
	klog.Infof("Creating ConfigMap %s/%s", ndb.Namespace, ndb.GetConfigMapName())

	cm = resources.GenerateConfigMapObject(ndb)
	cm, err = rcmc.k8client.CoreV1().ConfigMaps(ndb.Namespace).Create(cm)

	return cm, false, nil
}

// PatchConfigMap patches the existing config map with new configuration data generated from ndb CRD object
func (rcmc *ConfigMapControl) PatchConfigMap(ndb *v1alpha1.Ndb) (*corev1.ConfigMap, error) {

	// Get the StatefulSet with the name specified in Ndb.spec, fetching from client not cache
	cmOrg, err := rcmc.k8client.CoreV1().ConfigMaps(ndb.Namespace).Get(ndb.GetConfigMapName(), metav1.GetOptions{})

	// If the resource doesn't exist
	if errors.IsNotFound(err) {
	}

	cmChg := cmOrg.DeepCopy()
	cmChg = resources.InjectUpdateToConfigMapObject(ndb, cmChg)

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

		result, err = rcmc.k8client.CoreV1().ConfigMaps(ndb.Namespace).Patch(cmOrg.Name,
			types.StrategicMergePatchType,
			patchBytes)

		if err != nil {
			klog.Errorf("Failed to patch config map: %v", err)
			return false, err
		}

		return true, nil
	})

	return result, updateErr
}

// DeleteConfigMap -
// TODO make functions
func (rcmc *ConfigMapControl) DeleteConfigMap(ndb *v1alpha1.Ndb) error {
	panic("DeleteConfigMap not implemented yet")
	return nil
}
