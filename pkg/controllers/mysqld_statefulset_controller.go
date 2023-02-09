// Copyright (c) 2020, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"fmt"
	"strconv"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller"
	"github.com/mysql/ndb-operator/pkg/mysqlclient"
	"github.com/mysql/ndb-operator/pkg/resources"
	"github.com/mysql/ndb-operator/pkg/resources/statefulset"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	listersappsv1 "k8s.io/client-go/listers/apps/v1"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	klog "k8s.io/klog/v2"
)

const (
	// rootHost is the annotation key which stores the Root user's current host
	rootHost = ndbcontroller.GroupName + "/root-host"
	// rootUserGeneration is the annotation key which stores the NdbCluster
	// generation whose spec has been applied to the Root user.
	rootUserGeneration = ndbcontroller.GroupName + "/root-user-generation"
)

type mysqldStatefulSetController struct {
	ndbNodeStatefulSetImpl
}

// NewMySQLDStatefulSetController creates a new mysqldStatefulSetController
func newMySQLDStatefulSetController(
	client kubernetes.Interface,
	statefulSetLister listersappsv1.StatefulSetLister,
	configmapLister listerscorev1.ConfigMapLister) *mysqldStatefulSetController {
	return &mysqldStatefulSetController{
		ndbNodeStatefulSetImpl{
			client:             client,
			statefulSetLister:  statefulSetLister,
			ndbNodeStatefulset: statefulset.NewMySQLdStatefulSet(configmapLister),
		},
	}
}

// HandleScaleDown scales down the MySQL Server StatefulSet if it has been requested
// in the NdbCluster spec. This method is called before the config version is ensured
// in the management and data nodes, i.e., before any new config is applied to the
// management and data nodes. This is to ensure that during a scale down, the MySQL
// Servers are shutdown before a possible reduction in the number of API sections in
// the config.
func (mssc *mysqldStatefulSetController) HandleScaleDown(ctx context.Context, sc *SyncContext) syncResult {

	nc := sc.ndb
	mysqldSfset := sc.mysqldSfset

	if mysqldSfset == nil {
		// Nothing to scale down
		return continueProcessing()
	}
	NdbGeneration := sc.configSummary.NdbClusterGeneration
	// StatefulSet exists
	if !sc.isStatefulsetUpdated(sc.mysqldSfset, NdbGeneration, Complete) {
		// Previous StatefulSet update is not complete yet.
		// Finish processing. Reconciliation will
		// continue once the StatefulSet update has been
		// rolled out.
		return finishProcessing()
	}

	// Handle any scale down
	mysqldNodeCount := sc.configSummary.NumOfMySQLServers
	if mysqldSfset.Status.Replicas <= mysqldNodeCount {
		// No scale down requested or, it has been processed already
		// Continue processing rest of sync loop
		return continueProcessing()
	}

	// scale down requested
	if mysqldNodeCount == 0 {
		// The StatefulSet has to be deleted
		// Delete the root user first.
		rootHost := mysqldSfset.GetAnnotations()[rootHost]
		secretClient := NewMySQLUserPasswordSecretInterface(mssc.client)

		// Extract ndb operator mysql user password.
		operatorSecretName := resources.GetMySQLNDBOperatorPasswordSecretName(nc)
		operatorPassword, err := secretClient.ExtractPassword(ctx, mysqldSfset.Namespace, operatorSecretName)
		if err != nil {
			klog.Errorf("Failed to extract ndb operator password from the secret")
			return errorWhileProcessing(err)
		}

		if err := mysqlclient.DeleteRootUserIfExists(mysqldSfset, rootHost, operatorPassword); err != nil {
			klog.Errorf("Failed to delete root user")
			return errorWhileProcessing(err)
		}

		// Delete the secret.
		annotations := mysqldSfset.GetAnnotations()
		secretName := annotations[statefulset.RootPasswordSecret]

		if secretClient.IsControlledBy(ctx, secretName, nc) {
			// The given NdbCluster is set as the Owner of the secret,
			// which implies that this was created by the operator.
			err := secretClient.Delete(ctx, mysqldSfset.Namespace, secretName)
			if err != nil && !errors.IsNotFound(err) {
				// Delete failed with an error.
				// Ignore NotFound error as this delete might be a redundant
				// step, caused by an outdated cache read.
				klog.Errorf("Failed to delete MySQL Root pass secret %q : %s", secretName, err)
				return errorWhileProcessing(err)
			}
		}

		// scale down to 0 servers, i.e., delete the statefulset
		if err := mssc.deleteStatefulSet(ctx, mysqldSfset, sc); err != nil {
			return errorWhileProcessing(err)
		}

		// reconciliation will continue once the statefulset has been deleted
		return finishProcessing()
	}

	// create a new statefulset with updated replica to patch the original statefulset
	// Note : the annotation 'last-applied-config-generation' will be updated only
	//        during ReconcileStatefulset
	updatedSfset := mysqldSfset.DeepCopy()
	updatedSfset.Spec.Replicas = &mysqldNodeCount
	return mssc.patchStatefulSet(ctx, mysqldSfset, updatedSfset)
}

// ReconcileStatefulSet compares the MySQL Server spec defined in NdbCluster resource
// and applies any changes to the statefulset if required. This method is called after
// the new config has been ensured in both Management and Data Nodes.
func (mssc *mysqldStatefulSetController) ReconcileStatefulSet(ctx context.Context, sc *SyncContext) syncResult {
	mysqldSfset := sc.mysqldSfset
	cs := sc.configSummary
	nc := sc.ndb

	if mysqldSfset == nil {
		// statefulset doesn't exist yet
		if cs.NumOfMySQLServers == 0 {
			// the current state is in sync with expectation
			return continueProcessing()
		}

		// StatefulSet has to be created
		// First ensure that a root password secret exists
		secretClient := NewMySQLUserPasswordSecretInterface(mssc.client)
		if _, err := secretClient.EnsureMySQLRootPassword(ctx, nc); err != nil {
			klog.Errorf("Failed to ensure root password secret for StatefulSet %q : %s",
				mssc.ndbNodeStatefulset.GetName(nc), err)
			return errorWhileProcessing(err)
		}

		// create a statefulset
		if _, err := mssc.createStatefulSet(ctx, sc); err != nil {
			return errorWhileProcessing(err)
		}

		// StatefulSet was created successfully.
		// Finish processing. Reconciliation will
		// continue once the statefulset is updated.
		return finishProcessing()
	}

	// At this point the statefulset exists and has already been verified
	// to be complete (i.e. no previous updates still being applied) by HandleScaleDown.
	// Check if the statefulset has the recent config generation.
	if workloadHasConfigGeneration(mysqldSfset, cs.NdbClusterGeneration) {
		// Statefulset upto date
		klog.Info("All MySQL Servers are up-to-date and ready")
		return continueProcessing()
	}

	// Statefulset has to be patched
	// Patch the Governing Service first
	if err := sc.serviceController.patchService(ctx, sc, mssc.ndbNodeStatefulset); err != nil {
		return errorWhileProcessing(err)
	}

	// Patch the StatefulSet
	updatedStatefulSet, err := mssc.ndbNodeStatefulset.NewStatefulSet(cs, nc)
	if err != nil {
		return errorWhileProcessing(err)
	}

	return mssc.patchStatefulSet(ctx, mysqldSfset, updatedStatefulSet)
}

// reconcileRootUser creates or updates the root user with the recent NdbCluster spec
func (mssc *mysqldStatefulSetController) reconcileRootUser(ctx context.Context, sc *SyncContext) syncResult {
	mysqldSfset := sc.mysqldSfset
	if mysqldSfset == nil {
		// Nothing to do as the MySQL Servers do not exist
		return continueProcessing()
	}

	// Get the last applied NdbCluster Generation to the Root User
	annotations := mysqldSfset.GetAnnotations()
	var rootUserGen int64
	if genString, exists := annotations[rootUserGeneration]; exists {
		rootUserGen, _ = strconv.ParseInt(genString, 10, 64)
	}

	recentNdbGen := sc.configSummary.NdbClusterGeneration
	if rootUserGen == recentNdbGen {
		// The Root user spec is up-to-date
		return continueProcessing()
	}

	// The root user needs be created or updated
	nc := sc.ndb
	newRootHost := nc.Spec.MysqlNode.RootHost

	// Extract ndb operator mysql user password.
	secretClient := NewMySQLUserPasswordSecretInterface(sc.kubeClientset())
	operatorSecretName := resources.GetMySQLNDBOperatorPasswordSecretName(nc)
	operatorPassword, err := secretClient.ExtractPassword(ctx, mysqldSfset.Namespace, operatorSecretName)
	if err != nil {
		return errorWhileProcessing(err)
	}

	if existingRootHost, exists := annotations[rootHost]; !exists {
		// Root user doesn't exist yet - create it.
		// Extract root user password.
		secretName, _ := resources.GetMySQLRootPasswordSecretName(nc)
		rootPassword, err := secretClient.ExtractPassword(ctx, mysqldSfset.Namespace, secretName)
		if err != nil {
			return errorWhileProcessing(err)
		}

		// Create Root user
		if err = mysqlclient.CreateRootUserIfNotExist(mysqldSfset, newRootHost, rootPassword, operatorPassword); err != nil {
			klog.Errorf("Failed to create root user")
			return errorWhileProcessing(err)
		}
	} else if newRootHost != existingRootHost {
		// Root Host needs to be updated
		if err := mysqlclient.UpdateRootUser(mysqldSfset, existingRootHost, newRootHost, operatorPassword); err != nil {
			klog.Errorf("Failed to update root user")
			return errorWhileProcessing(err)
		}
	}

	// Successfully applied the changes to root user
	// Patch the StatefulSet to mark the changes as done
	updatedMysqldSfset := mysqldSfset.DeepCopy()
	annotations = updatedMysqldSfset.Annotations
	annotations[rootHost] = newRootHost
	annotations[rootUserGeneration] = fmt.Sprintf("%d", recentNdbGen)
	return mssc.patchStatefulSet(ctx, mysqldSfset, updatedMysqldSfset)
}
