// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/resources"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	typedappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	"k8s.io/klog"
)

// deploymentComplete considers a deployment to be complete once all of its desired replicas
// are updated and available, and no old pods are running.
func deploymentComplete(deployment *appsv1.Deployment) bool {
	return deployment.Status.UpdatedReplicas == *(deployment.Spec.Replicas) &&
		deployment.Status.Replicas == *(deployment.Spec.Replicas) &&
		deployment.Status.AvailableReplicas == *(deployment.Spec.Replicas) &&
		deployment.Status.ObservedGeneration >= deployment.Generation
}

// deploymentHasConfig returns true if the given deployment has the expected config generation
func deploymentHasConfig(deployment *appsv1.Deployment, expectedConfigGeneration uint32) bool {
	// Get the last applied Config Generation
	annotations := deployment.Spec.Template.GetAnnotations()
	existingConfigGeneration, _ := strconv.ParseUint(annotations[resources.LastAppliedConfigGeneration], 10, 64)
	return uint32(existingConfigGeneration) == expectedConfigGeneration
}

// DeploymentControlInterface is the interface for deployment controllers
type DeploymentControlInterface interface {
	GetTypeName() string
	GetDeployment(ctx context.Context, nc *v1alpha1.NdbCluster) (*appsv1.Deployment, error)
	HandleScaleDown(ctx context.Context, sc *SyncContext) syncResult
	ReconcileDeployment(ctx context.Context, sc *SyncContext) syncResult
}

type mysqlDeploymentController struct {
	client                kubernetes.Interface
	mysqlServerDeployment *resources.MySQLServerDeployment
}

// NewMySQLDeploymentController returns a new mysqlDeploymentController
func NewMySQLDeploymentController(client kubernetes.Interface, nc *v1alpha1.NdbCluster) DeploymentControlInterface {
	return &mysqlDeploymentController{
		client:                client,
		mysqlServerDeployment: resources.NewMySQLServerDeployment(),
	}
}

// deploymentInterface returns a typed/apps/v1.DeploymentInterface
func (mdc *mysqlDeploymentController) deploymentInterface(namespace string) typedappsv1.DeploymentInterface {
	return mdc.client.AppsV1().Deployments(namespace)
}

// GetTypeName returns the type of the resource being
// controlled by the DeploymentControlInterface
func (mdc *mysqlDeploymentController) GetTypeName() string {
	return mdc.mysqlServerDeployment.GetTypeName()
}

// GetDeployment retrieves the MySQL deployment for the given NdbCluster resource
func (mdc *mysqlDeploymentController) GetDeployment(
	ctx context.Context, nc *v1alpha1.NdbCluster) (*appsv1.Deployment, error) {
	deployment, err := mdc.deploymentInterface(nc.Namespace).Get(
		ctx, mdc.mysqlServerDeployment.GetName(nc), metav1.GetOptions{})

	if err != nil && !errors.IsNotFound(err) {
		klog.Errorf("Failed to retrieve the deployment for NdbCluster %q : %s", nc.Name, err)
		return nil, err
	}

	if errors.IsNotFound(err) {
		return nil, nil
	}

	return deployment, nil
}

// createDeployment ensures the MySQL Server root password and then creates the deployment of MySQL Servers.
func (mdc *mysqlDeploymentController) createDeployment(ctx context.Context, sc *SyncContext) error {

	// First ensure that a root password secret exists
	secretClient := NewMySQLRootPasswordSecretInterface(mdc.client)
	if _, err := secretClient.Ensure(ctx, sc.ndb); err != nil {
		klog.Errorf("Failed to ensure root password secret for deployment %q : %s",
			mdc.mysqlServerDeployment.GetName(sc.ndb), err)
		return err
	}

	// Create deployment
	deployment := mdc.mysqlServerDeployment.NewDeployment(sc.ndb, sc.resourceContext, nil)
	_, err := mdc.deploymentInterface(deployment.Namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		// Creating deployment failed
		klog.Errorf("Failed to create deployment %q : %s", getNamespacedName(deployment), err)
		return err
	}

	// New deployment was successfully created
	klog.Errorf("Created the MySQL Server deployment %q", getNamespacedName(deployment))
	return nil

}

// DeleteDeployment deletes the given deployment and any associated secret created by the operator
func (mdc *mysqlDeploymentController) deleteDeployment(
	ctx context.Context, deployment *appsv1.Deployment, nc *v1alpha1.NdbCluster) error {
	// Delete the secret before deleting the deployment
	annotations := deployment.GetAnnotations()
	secretName := annotations[resources.RootPasswordSecret]
	secretClient := NewMySQLRootPasswordSecretInterface(mdc.client)
	if secretClient.IsControlledBy(ctx, secretName, nc) {
		// The given NdbCluster is set as the Owner of the secret,
		// which implies that this was created by the operator.
		err := secretClient.Delete(ctx, deployment.Namespace, secretName)
		if err != nil {
			klog.Errorf("Failed to delete MySQL Root pass secret %q : %s", secretName, err)
			return err
		}
	}

	// delete the deployment
	err := mdc.deploymentInterface(deployment.Namespace).Delete(
		context.TODO(), deployment.Name, metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("Failed to delete the deployment %q : %s", getNamespacedName(deployment), err)
		return err
	}
	klog.Errorf("Deleted the deployment %q", getNamespacedName(deployment))
	return nil
}

// patchDeployment generates and applies the patch to the deployment
func (mdc *mysqlDeploymentController) patchDeployment(
	existingDeployment *appsv1.Deployment, updatedDeployment *appsv1.Deployment) syncResult {
	// JSON encode both deployments
	existingJSON, err := json.Marshal(existingDeployment)
	if err != nil {
		klog.Errorf("Failed to encode existing deployment: %v", err)
		return errorWhileProcessing(err)
	}
	updatedJSON, err := json.Marshal(updatedDeployment)
	if err != nil {
		klog.Errorf("Failed to encode updated deployment: %v", err)
		return errorWhileProcessing(err)
	}

	// Generate the patch to be applied
	patch, err := strategicpatch.CreateTwoWayMergePatch(existingJSON, updatedJSON, appsv1.Deployment{})
	if err != nil {
		klog.Errorf("Failed to generate the patch to be applied: %v", err)
		return errorWhileProcessing(err)
	}

	// klog.Infof("Patching deployments.\nExisting : %v\n. Modified : %v\nPatch : %v", string(existingJSON), string(updatedJSON), string(patch))

	// Patch the deployment
	deploymentInterface := mdc.deploymentInterface(existingDeployment.Namespace)
	deployment, err := deploymentInterface.Patch(
		context.TODO(), existingDeployment.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		klog.Errorf("Failed to apply the patch to the deployment %q : %s", getNamespacedName(existingDeployment), err)
		return errorWhileProcessing(err)
	}

	// successfully applied the patch
	klog.Infof("Deployment %q has been patched successfully", getNamespacedName(deployment))
	return requeueInSeconds(5)
}

// HandleScaleDown scales down the MySQL deployment if it has been requested in the NdbCluster spec.
// This method is called before the config version is ensured in the management and data nodes,
// i.e., before any new config is applied to the management and data nodes. This is to ensure that
// during a scale down, the MySQL Servers are shutdown before a possible reduction in the number of
// API sections in the config.
func (mdc *mysqlDeploymentController) HandleScaleDown(ctx context.Context, sc *SyncContext) syncResult {

	ndbCluster := sc.ndb
	deployment := sc.mysqldDeployment

	if deployment == nil {
		// Nothing to scale down
		return continueProcessing()
	}

	// Deployment exists
	if !deploymentComplete(deployment) {
		// Previous deployment not complete yet. Return and requeue.
		return requeueInSeconds(5)
	}

	// Handle any scale down
	mysqldNodeCount := int32(sc.resourceContext.NumOfMySQLServers)
	if deployment.Status.Replicas <= mysqldNodeCount {
		// No scale down requested or, it has been processed already
		// Continue processing rest of sync loop
		return continueProcessing()
	}

	// scale down requested
	if mysqldNodeCount == 0 {
		// scale down to 0 servers; delete the deployment
		if err := mdc.deleteDeployment(ctx, deployment, ndbCluster); err != nil {
			return errorWhileProcessing(err)
		}
		return requeueInSeconds(0)
	}

	// create a new deployment with updated replica to patch the original deployment
	// Note : the annotation 'last-applied-config-generation' will be updated only
	//        during ReconcileDeployment
	updatedDeployment := deployment.DeepCopy()
	updatedDeployment.Spec.Replicas = &mysqldNodeCount
	return mdc.patchDeployment(deployment, updatedDeployment)

}

// ReconcileDeployment compares the MySQL Server spec defined in NdbCluster resource
// and applies any changes to the deployment if required. This method is called after
// the new config has been ensured in both Management and Data Nodes.
func (mdc *mysqlDeploymentController) ReconcileDeployment(ctx context.Context, sc *SyncContext) syncResult {
	deployment := sc.mysqldDeployment
	rc := sc.resourceContext
	ndbCluster := sc.ndb

	if deployment == nil {
		// deployment doesn't exist yet
		if rc.NumOfMySQLServers == 0 {
			// the current state is in sync with expectation
			return continueProcessing()
		}

		// create a deployment
		if err := mdc.createDeployment(ctx, sc); err != nil {
			return errorWhileProcessing(err)
		}

		// deployment was created successfully.
		// Wait for it to become ready
		return requeueInSeconds(5)
	}

	// At this point the deployment exists and has already been verified
	// to be complete (i.e. no previous updates still being applied) by HandleScaleDown.
	// Check if it has the recent config generation.
	if deploymentHasConfig(deployment, rc.ConfigGeneration) {
		// Deployment upto date
		return continueProcessing()
	}

	// Deployment has to be patched
	updatedDeployment := mdc.mysqlServerDeployment.NewDeployment(ndbCluster, rc, deployment)
	return mdc.patchDeployment(deployment, updatedDeployment)
}
