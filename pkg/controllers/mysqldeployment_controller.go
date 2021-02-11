// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/resources"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
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

type DeploymentControlInterface interface {
	GetTypeName() string
	EnsureDeployment(ndb *v1alpha1.Ndb, rc *resources.ResourceContext) (*appsv1.Deployment, bool, error)
	ReconcileDeployment(ndb *v1alpha1.Ndb,
		deployment *appsv1.Deployment, rc *resources.ResourceContext, handleScaleDown bool) syncResult
}

type mysqlDeploymentController struct {
	client                kubernetes.Interface
	deploymentLister      appslisters.DeploymentLister
	mysqlServerDeployment *resources.MySQLServerDeployment
}

// GetTypeName returns the type of the resource being
// controlled by the DeploymentControlInterface
func (mdc *mysqlDeploymentController) GetTypeName() string {
	return mdc.mysqlServerDeployment.GetTypeName()
}

// ensureRootPasswordSecret ensures the existence of a root-password-secret by creating one if it doesn't exist
func (mdc *mysqlDeploymentController) ensureRootPasswordSecret(ndb *v1alpha1.Ndb) (err error) {
	secretName := mdc.mysqlServerDeployment.GetRootPasswordSecretName(ndb)
	secretInterface := mdc.client.CoreV1().Secrets(ndb.Namespace)
	if _, err = secretInterface.Get(context.TODO(), secretName, metav1.GetOptions{}); err == nil {
		// Secret exists
		return nil
	}

	if !errors.IsNotFound(err) {
		// Error retrieving the secret
		klog.Errorf("Failed to ensure secret %s : %v", secretName, err)
		return err
	}

	// Create secret
	secret := mdc.mysqlServerDeployment.NewMySQLRootPasswordSecret(ndb)
	if _, err = secretInterface.Create(context.TODO(), secret, metav1.CreateOptions{}); err != nil {
		klog.Errorf("Failed to create secret %s : %v", secretName, err)
	}

	return err
}

// createDeployment takes the representation of a deployment and creates it
func (mdc *mysqlDeploymentController) createDeployment(deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	return mdc.client.AppsV1().Deployments(deployment.Namespace).Create(
		context.TODO(), deployment, metav1.CreateOptions{})
}

// patchDeployment generates and applies the patch to the deployment
func (mdc *mysqlDeploymentController) patchDeployment(
	existingDeployment *appsv1.Deployment, updatedDeployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	// JSON encode both deployments
	existingJSON, err := json.Marshal(existingDeployment)
	if err != nil {
		klog.Errorf("Failed to encode existing deployment: %v", err)
		return nil, err
	}
	updatedJSON, err := json.Marshal(updatedDeployment)
	if err != nil {
		klog.Errorf("Failed to encode updated deployment: %v", err)
		return nil, err
	}

	// Generate the patch to be applied
	patch, err := strategicpatch.CreateTwoWayMergePatch(existingJSON, updatedJSON, appsv1.Deployment{})
	if err != nil {
		klog.Errorf("Failed to generate the patch to be applied: %v", err)
		return nil, err
	}

	// klog.Infof("Patching deployments.\nExisting : %v\n. Modified : %v\nPatch : %v", string(existingJSON), string(updatedJSON), string(patch))

	// Patch the deployment
	deploymentInterface := mdc.client.AppsV1().Deployments(existingDeployment.Namespace)
	deployment, err := deploymentInterface.Patch(
		context.TODO(), existingDeployment.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		klog.Errorf("Failed to apply the patch to the deployment '%s': %v", existingDeployment.Name, err)
		return nil, err
	}

	// successfully applied the patch
	klog.Infof("Deployment '%s' has been patched successfully", deployment.Name)
	return deployment, nil
}

// ReconcileDeployment compares the MySQL Server spec defined
// in Ndb resource and makes changes to the deployment if required
//
// Note : This function is called twice by the sync function.
//        Once before the config version is ensured in the management
//        and data nodes and once after they are ensured. During the
//        first pass, only scale down, if any, is handled. Any other
//        changes to the template and scaling up are handled during
//        the second pass. This is to ensure that during a scale down,
//        the Servers are shutdown before a possible reduction in the
//        number of API sections in the config.
func (mdc *mysqlDeploymentController) ReconcileDeployment(
	ndb *v1alpha1.Ndb, deployment *appsv1.Deployment, rc *resources.ResourceContext, handleScaleDown bool) syncResult {

	// Nothing to reconcile if there is no existing deployment
	if deployment == nil {
		// TODO interesting case - we should not never be here
		// if there is no deployment then it hasn't been created
		// should it be created even if replicas == 0 in MysqldSpec?
		klog.Warningf("MySQL Server deployment does not exist")
		return continueProcessing()
	}

	if !deploymentComplete(deployment) {
		// Previous deployment not complete yet. Return and requeue.
		return requeueInSeconds(1)
	}

	// Get the last applied Config Generation
	annotations := deployment.GetAnnotations()
	existingConfigGeneration, _ := strconv.ParseInt(annotations[constants.LastAppliedConfigGeneration], 10, 64)
	if existingConfigGeneration == rc.ConfigGeneration {
		// Deployment upto date with the current config
		return continueProcessing()
	}

	// Handle spec/config change
	var updatedDeployment *appsv1.Deployment
	mysqldNodeCount := ndb.GetMySQLServerNodeCount()
	if handleScaleDown {
		// First pass - handle only the scale down, if any
		if deployment.Status.Replicas <= mysqldNodeCount {
			// No scale down requested or it has been processed already
			// Continue processing rest of sync loop
			return continueProcessing()
		}
		// scale down requested
		// create a new deployment with updated replica to patch the original deployment
		// Note : the annotation 'last-applied-config-generation' will be updated only
		//        during the second pass.
		updatedDeployment = deployment.DeepCopy()
		updatedDeployment.Spec.Replicas = &mysqldNodeCount
	} else {
		// Second pass - patch in the any other spec changes and scale up
		updatedDeployment = mdc.mysqlServerDeployment.NewDeployment(ndb, rc, deployment)
	}

	patchedDeployment, err := mdc.patchDeployment(deployment, updatedDeployment)
	if err != nil {
		klog.Errorf("Failed to patch MySQL Server deployment")
		return errorWhileProcssing(err)
	}

	// If any change has been done to the spec of the deployment, exit and continue later
	if !deploymentComplete(patchedDeployment) {
		return requeueInSeconds(5)
	}

	// No change to deployment spec - continue processing
	return continueProcessing()
}

// EnsureDeployment checks if the MySQLServerDeployment already
// exists. If not, it creates a new deployment.
func (mdc *mysqlDeploymentController) EnsureDeployment(
	ndb *v1alpha1.Ndb, rc *resources.ResourceContext) (*appsv1.Deployment, bool, error) {

	// Get the deployment in the namespace of Ndb resource
	// with the name matching that of MySQLServerDeployment
	deployment, err := mdc.deploymentLister.Deployments(ndb.Namespace).Get(mdc.mysqlServerDeployment.GetName())

	// Return if the deployment exists already or
	// if listing failed due to some other error than not found
	if err == nil || !errors.IsNotFound(err) {
		return deployment, err == nil, err
	}

	// Deployment doesn't exist
	// First ensure that a root password secret exists
	if err = mdc.ensureRootPasswordSecret(ndb); err != nil {
		return nil, false, err
	}

	// Create deployment
	numberOfMySQLServers := ndb.GetMySQLServerNodeCount()
	klog.Infof("Creating a deployment of '%d' MySQL Servers", numberOfMySQLServers)
	deployment = mdc.mysqlServerDeployment.NewDeployment(ndb, rc, nil)
	if _, err = mdc.createDeployment(deployment); err != nil {
		// Creating deployment failed
		klog.Errorf("Failed to create deployment of '%d' MySQL Servers with error: %s",
			numberOfMySQLServers, err)
		return nil, false, err
	}

	// New deployment was successfully created
	return deployment, false, nil
}
