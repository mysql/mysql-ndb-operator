// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"encoding/json"

	"github.com/mysql/ndb-operator/pkg/resources"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
)

type ConfigMapControlInterface interface {
	EnsureConfigMap(ctx context.Context, sc *SyncContext) (*corev1.ConfigMap, bool, error)
	PatchConfigMap(ctx context.Context, sc *SyncContext) (*corev1.ConfigMap, error)
}

type configMapControl struct {
	k8sClient kubernetes.Interface
}

// NewConfigMapControl creates a new ConfigMapControl
func NewConfigMapControl(client kubernetes.Interface) ConfigMapControlInterface {
	return &configMapControl{
		k8sClient: client,
	}
}

func (cmc *configMapControl) getConfigMapInterface(namespace string) typedcorev1.ConfigMapInterface {
	return cmc.k8sClient.CoreV1().ConfigMaps(namespace)
}

// getConfigMap retrieves the ConfigMap from the API Server
func (cmc *configMapControl) getConfigMap(
	ctx context.Context, namespace, name string) (*corev1.ConfigMap, error) {
	// Cache is not used as it might be outdated and all
	// other resources depend on the ConfigMap being right.
	configMapInterface := cmc.getConfigMapInterface(namespace)
	return configMapInterface.Get(ctx, name, metav1.GetOptions{})

}

// EnsureConfigMap creates a config map for the NdbCluster resource if one does not exist already
func (cmc *configMapControl) EnsureConfigMap(
	ctx context.Context, sc *SyncContext) (cm *corev1.ConfigMap, existed bool, err error) {

	// Retrieve the ConfigMap directly from the K8s API Server and not from cache.
	nc := sc.ndb
	configMapName := nc.GetConfigMapName()
	cm, err = cmc.getConfigMap(ctx, nc.Namespace, configMapName)

	if err == nil {
		// ConfigMap already exists
		if err = sc.isOwnedByNdbCluster(cm); err != nil {
			// But is not owned by NdbCluster resource
			return nil, false, err
		}

		// ConfigMap exists and is owned by the NdbCluster resource
		return cm, true, nil
	}

	if !errors.IsNotFound(err) {
		// Failed to lookup ConfigMap
		klog.Errorf("Failed to retrieve ConfigMap \"%s/%s\"", nc.Namespace, configMapName)
		return nil, false, err
	}

	// ConfigMap doesn't exist; create it.
	klog.Infof("Creating ConfigMap \"%s/%s\"", nc.Namespace, configMapName)
	cm = resources.CreateConfigMap(nc)
	cm, err = cmc.getConfigMapInterface(nc.Namespace).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		klog.Errorf("Failed to create ConfigMap \"%s/%s\" : %s", nc.Namespace, configMapName, err)
		return nil, false, err
	}

	// ConfigMap was created
	return cm, false, nil
}

// PatchConfigMap patches the existing config map with new configuration data generated from ndb CRD object
func (cmc *configMapControl) PatchConfigMap(
	ctx context.Context, sc *SyncContext) (cm *corev1.ConfigMap, err error) {

	// Retrieve the ConfigMap directly from the K8s API Server and not from cache.
	nc := sc.ndb
	configMapName := nc.GetConfigMapName()
	cmOrg, err := cmc.getConfigMap(ctx, nc.Namespace, configMapName)
	if err != nil {
		klog.Errorf("Error retrieving ConfigMap \"%s/%s\" : %s", nc.Namespace, configMapName, err)
		return nil, err
	}

	// Get an updated config map copy
	cs := sc.configSummary
	cmChg := resources.GetUpdatedConfigMap(nc, cmOrg, cs)

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
	ConfigMapInterface := cmc.getConfigMapInterface(nc.Namespace)
	// Patch the ConfigMap with retries on failure
	updateErr := wait.ExponentialBackoff(retry.DefaultBackoff, func() (ok bool, err error) {

		result, err = ConfigMapInterface.Patch(
			ctx, cmOrg.Name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})

		if err != nil {
			klog.Errorf("Failed to patch ConfigMap \"%s/%s\" : %s",
				nc.Namespace, configMapName, err)
			return false, err
		}

		return true, nil
	})

	klog.Infof("Successfully patched ConfigMap %q", getNamespacedName(cmChg))
	return result, updateErr
}
