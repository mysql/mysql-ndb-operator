// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// NOTE: NOT READY AT ALL - FIX BUT DON'T USE

package controllers

import (
	"testing"

	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
)

func TestCreateConfigMap(t *testing.T) {

	f := newFixture(t)
	defer f.close()

	ndb := newNdb("test", 1)

	// we first need to set up arrays with objects ...
	f.ndbLister = append(f.ndbLister, ndb)
	f.objects = append(f.objects, ndb)

	// ... before we init the fake clients with those objects.
	// objects not listed in arrays at fakeclient setup will eventually be deleted
	f.init()

	cmc := NewConfigMapControl(f.kubeclient, f.k8If.Core().V1().ConfigMaps())

	f.start()

	cm, err := cmc.EnsureConfigMap(ndb)

	if err != nil {
		t.Errorf("Unexpected error EnsuringConfigMap: %v", err)
	}
	if cm == nil {
		t.Errorf("Unexpected error EnsuringConfigMap: return null pointer")
	}

	f.expectCreateAction(ndb.GetNamespace(), "configmap", cm)

	rcmc := cmc.(*ConfigMapControl)

	// Wait for the caches to be synced before using Lister to get new config map
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(f.stopCh, rcmc.configMapListerSynced); !ok {
		t.Errorf("failed to wait for caches to sync")
		return
	}

	// Get the StatefulSet with the name specified in Ndb.spec
	cmget, err := rcmc.configMapLister.ConfigMaps(ndb.Namespace).Get(configMapName)
	if err != nil {
		t.Errorf("Unexpected error getting created ConfigMap: %v", err)
	}
	if cmget == nil {
		t.Errorf("Unexpected error EnsuringConfigMap: didn't find created ConfigMap")
	}

}
