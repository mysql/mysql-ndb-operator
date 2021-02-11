// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// NOTE: NOT READY AT ALL - FIX BUT DON'T USE

package controllers

import (
	"encoding/json"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

func Test_TestThingsrelatedToConfigMaps(t *testing.T) {

	configString := "Version 1"
	ns := metav1.NamespaceDefault

	d1 := map[string]string{
		"config.ini": configString,
	}
	d2 := map[string]string{
		"config.ini": "Version 2",
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "configtest",
			Namespace: ns,
		},
		Data: d1,
	}

	cm2 := cm.DeepCopy()

	j, err := json.Marshal(cm)
	if err != nil {
		t.Error(err)
	}

	cm2.Data = d2

	j2, err := json.Marshal(cm2)
	if err != nil {
		t.Error(err)
	}
	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(
		j, j2, corev1.ConfigMap{})
	if err != nil {
		t.Error(err)
	}

	fmt.Println(string(patchBytes))

	t.Fail()
}

func TestCreateConfigMap(t *testing.T) {

	f := newFixture(t)
	defer f.close()

	ns := metav1.NamespaceDefault
	ndb := newNdb(ns, "test", 1)
	ndb.Spec.Mysqld.NodeCount = 7

	// we first need to set up arrays with objects ...
	f.ndbLister = append(f.ndbLister, ndb)
	f.objects = append(f.objects, ndb)

	// ... before we init the fake clients with those objects.
	// objects not listed in arrays at fakeclient setup will eventually be deleted
	f.init()

	cmc := NewConfigMapControl(f.kubeclient, f.k8If.Core().V1().ConfigMaps())

	f.start()

	cm, existed, err := cmc.EnsureConfigMap(ndb)

	if err != nil {
		t.Errorf("Unexpected error EnsuringConfigMap: %v", err)
	}
	if cm == nil {
		t.Errorf("Unexpected error EnsuringConfigMap: return null pointer")
	}
	if existed {
		t.Errorf("Unexpected error EnsuringConfigMap: should not have existed")
	}

	f.expectCreateAction(ndb.GetNamespace(), "", "v1", "configmap", cm)

	rcmc := cmc.(*ConfigMapControl)

	// Wait for the caches to be synced before using Lister to get new config map
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(f.stopCh, rcmc.configMapListerSynced); !ok {
		t.Errorf("failed to wait for caches to sync")
		return
	}

	// Get the StatefulSet with the name specified in Ndb.spec
	cmget, err := rcmc.configMapLister.ConfigMaps(ndb.Namespace).Get(ndb.GetConfigMapName())
	if err != nil {
		t.Errorf("Unexpected error getting created ConfigMap: %v", err)
	}
	if cmget == nil {
		t.Errorf("Unexpected error EnsuringConfigMap: didn't find created ConfigMap")
	}

	ndb.Spec.Mysqld.NodeCount = 12

	cm, err = cmc.PatchConfigMap(ndb)

	s, _ := json.MarshalIndent(cm, "", "  ")

	t.Errorf(string(s))
}
