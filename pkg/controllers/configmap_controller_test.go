// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// NOTE: NOT READY AT ALL - FIX BUT DON'T USE

package controllers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/helpers"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
)

func Test_TestThingsRelatedToConfigMaps(t *testing.T) {

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
	err = json.Unmarshal(patchBytes, &patchedCM)
	if err != nil {
		t.Error(err)
	}

	// Verify that patching worked as expected
	if len(patchedCM) != 1 || len(patchedCM["data"]) != 1 ||
		patchedCM["data"]["config.ini"] != "Version 2" {
		t.Logf("Patched cm : %s", string(patchBytes))
		t.Error("Patching failed")
	}
}

// ValidateConfigIniSectionCount validates the count of a
// given section in the configIni
func validateConfigIniSectionCount(t *testing.T, config helpers.ConfigIni, sectionName string, expected int) {
	t.Helper()
	if actual := config.GetNumberOfSections(sectionName); actual != expected {
		t.Errorf("Expected number of '%s' sections : %d. Actual : %d", sectionName, expected, actual)
	}
}

// validateMgmtConfig validates the config.ini key of the config map
func validateMgmtConfig(t *testing.T, cm *corev1.ConfigMap, ndb *v1alpha1.NdbCluster) {
	t.Helper()

	if cm == nil {
		t.Error("Config map is empty")
		return
	}

	// Parse the config.ini in the ConfigMap into a ConfigIni
	cfg, err := helpers.ParseString(cm.Data["config.ini"])
	if err != nil {
		t.Errorf("Parsing of config.ini from config map failed: %s", err)
		return
	}

	// Validate the number of sections
	validateConfigIniSectionCount(t, cfg, "ndb_mgmd", int(ndb.GetManagementNodeCount()))
	validateConfigIniSectionCount(t, cfg, "ndbd", int(ndb.Spec.NodeCount))
	validateConfigIniSectionCount(t, cfg, "api", int(ndb.GetMySQLServerNodeCount())+3)
}

func TestCreateConfigMap(t *testing.T) {

	ns := metav1.NamespaceDefault
	ndb := testutils.NewTestNdb(ns, "test", 2)
	ndb.Spec.Mysqld.NodeCount = 7

	f := newFixture(t, ndb)
	defer f.close()

	cmc := NewConfigMapControl(f.kubeclient)

	f.setupController("ndb-operator", true)
	sc := f.c.newSyncContext(ndb)

	cm, existed, err := cmc.EnsureConfigMap(context.TODO(), sc)
	f.expectCreateAction(ndb.GetNamespace(), "", "v1", "configmaps", cm)

	if err != nil {
		t.Fatalf("Unexpected error EnsuringConfigMap: %v", err)
	}
	if cm == nil {
		t.Fatalf("Unexpected error EnsuringConfigMap: return null pointer")
	}
	if existed {
		t.Errorf("Unexpected error EnsuringConfigMap: should not have existed")
	}

	// Validate cm
	validateMgmtConfig(t, cm, ndb)

	// Verify that EnsureConfigMap returns properly when the config map exists already
	// No Action is expected
	cmget, existed, err := cmc.EnsureConfigMap(context.TODO(), sc)
	if err != nil {
		t.Errorf("Unexpected error EnsuringConfigMap: %v", err)
	}
	if !existed || cmget == nil {
		t.Errorf("Unexpected error EnsuringConfigMap: config map didn't exist")
	}

	// Patch cmget and verify
	ndb.Spec.Mysqld.NodeCount = 12
	patchedCm, err := cmc.PatchConfigMap(context.TODO(), sc)
	if err != nil {
		t.Fatal("Unexpected error patching config map :", err)
	}
	// Passing nil as expected patch to skip comparing the expected and original patches
	f.expectPatchAction(ndb.GetNamespace(), "configmaps",
		cm.GetName(), types.StrategicMergePatchType, nil)

	// Validate patched cm
	validateMgmtConfig(t, patchedCm, ndb)

	// Validate all actions
	f.checkActions()
}
