// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// NOTE: NOT READY AT ALL - FIX BUT DON'T USE

package controllers

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/config"
	"github.com/mysql/ndb-operator/pkg/helpers"

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

	var patchedCM map[string]map[string]string
	json.Unmarshal(patchBytes, &patchedCM)

	// Verify that patching worked as expected
	if len(patchedCM) != 1 || len(patchedCM["data"]) != 1 ||
		patchedCM["data"]["config.ini"] != "Version 2" {
		t.Logf("Patched cm : %s", string(patchBytes))
		t.Error("Patching failed")
	}
}

// ValidateConfigIniSectionCount validates the count of a
// given section in the configIni
func validateConfigIniSectionCount(t *testing.T, config *helpers.ConfigIni, sectionName string, expected int) {
	t.Helper()
	if actual := helpers.GetNumberOfSectionsInSectionGroup(config, sectionName); actual != expected {
		t.Errorf("Expected number of '%s' sections : %d. Actual : %d", sectionName, expected, actual)
	}
}

// validateMgmtConfig validates the config.ini key of the config map
func validateMgmtConfig(t *testing.T, cm *corev1.ConfigMap, ndb *v1alpha1.Ndb) {
	t.Helper()

	if cm == nil {
		t.Error("Config map is empty")
		return
	}

	// Parse the config.ini in the ConfigMap into a ConfigIni
	config, err := helpers.ParseString(cm.Data["config.ini"])
	if err != nil {
		t.Errorf("Parsing of config.ini from config map failed: %s", err)
		return
	}

	// Validate the number of sections
	validateConfigIniSectionCount(t, config, "ndb_mgmd", ndb.GetManagementNodeCount())
	validateConfigIniSectionCount(t, config, "ndbd", int(*ndb.Spec.NodeCount))
	validateConfigIniSectionCount(t, config, "mysqld", int(ndb.GetMySQLServerNodeCount()))
}

func TestCreateConfigMap(t *testing.T) {

	ns := metav1.NamespaceDefault
	ndb := helpers.NewTestNdb(ns, "test", 2)
	ndb.Spec.Mysqld.NodeCount = 7

	f := newFixture(t, ndb)
	defer f.close()

	cmc := NewConfigMapControl(f.kubeclient, f.k8If.Core().V1().ConfigMaps())

	f.setupController("ndb-operator", true)
	sc := f.c.newSyncContext(ndb)

	config.ScriptsDir = filepath.Join("..", "helpers", "scripts")
	cm, existed, err := cmc.EnsureConfigMap(sc)

	if err != nil {
		t.Errorf("Unexpected error EnsuringConfigMap: %v", err)
	}
	if cm == nil {
		t.Errorf("Unexpected error EnsuringConfigMap: return null pointer")
	}
	if existed {
		t.Errorf("Unexpected error EnsuringConfigMap: should not have existed")
	}

	// Validate cm
	validateMgmtConfig(t, cm, ndb)

	f.expectCreateAction(ndb.GetNamespace(), "", "v1", "configmap", cm)

	rcmc := cmc.(*ConfigMapControl)

	// Wait for the caches to be synced before using Lister to get new config map
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(f.stopCh, rcmc.configMapListerSynced); !ok {
		t.Errorf("failed to wait for caches to sync")
		return
	}

	// Get the config map with the name specified in Ndb.spec
	cmget, err := rcmc.configMapLister.ConfigMaps(ndb.Namespace).Get(ndb.GetConfigMapName())
	if err != nil {
		t.Errorf("Unexpected error getting created ConfigMap: %v", err)
	}
	if cmget == nil {
		t.Errorf("Unexpected error EnsuringConfigMap: didn't find created ConfigMap")
	}

	// Validate cmget
	validateMgmtConfig(t, cmget, ndb)

	// Patch cmget and verify
	ndb.Spec.Mysqld.NodeCount = 12
	patchedCm, err := cmc.PatchConfigMap(ndb)

	// Validate patched cm
	validateMgmtConfig(t, patchedCm, ndb)
}
