// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"github.com/ocklin/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	corev1 "k8s.io/api/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
)

const configMapName = "ndb-config-ini"

type ConfigMapControlInterface interface {
	EnsureConfigMap(ndb *v1alpha1.Ndb) (*corev1.ConfigMap, error)
	UpdateConfigMap(ndb *v1alpha1.Ndb) (*corev1.ConfigMap, error)
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

func createConfigMapObject(ndb *v1alpha1.Ndb) *corev1.ConfigMap {

	/*
		kind: ConfigMap
		apiVersion: v1
		metadata:
			name: config-ini
			namespace: default
			#uid: a7ec90d9-f2a9-11ea-95f5-000d3a2ebd7f
			#resourceVersion: '58127766'
			#creationTimestamp: '2020-09-09T14:35:06Z'
		data:
			config.ini: |
				[DB DEFAULT]
				NoOfReplicas=2
				DataMemory=100M

				[TCP DEFAULT]
				AllowUnresolvedHostnames=true
				SendBufferMemory=64M
				ReceiveBufferMemory=8M


	*/

	configStr := `
	[DB DEFAULT]	
	NoOfReplicas=2
	DataMemory=100M
	`

	data := map[string]string{
		"config.ini": configStr,
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: ndb.Namespace,
		},
		Data: data,
	}

	return cm
}

func (rcmc *ConfigMapControl) EnsureConfigMap(ndb *v1alpha1.Ndb) (*corev1.ConfigMap, error) {

	// Get the StatefulSet with the name specified in Ndb.spec
	cm, err := rcmc.configMapLister.ConfigMaps(ndb.Namespace).Get(configMapName)

	// If the resource doesn't exist, we'll create it
	if errors.IsNotFound(err) {
		klog.Infof("Creating ConfigMap %s/%s", ndb.Namespace, configMapName)

		cm = createConfigMapObject(ndb)
		cm, err = rcmc.k8client.CoreV1().ConfigMaps(ndb.Namespace).Create(cm)
	}
	return cm, err
}

func (rcmc *ConfigMapControl) UpdateConfigMap(ndb *v1alpha1.Ndb) (*corev1.ConfigMap, error) {
	return nil, nil
}

func (rcmc *ConfigMapControl) DeleteConfigMap(ndb *v1alpha1.Ndb) error {
	return nil
}
