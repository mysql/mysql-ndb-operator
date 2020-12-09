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
	EnsureDeployment(ndb *v1alpha1.Ndb, rc *resources.ResourceContext) (*appsv1.Deployment, bool, error)
	ReconcileDeployment(ndb *v1alpha1.Ndb, deployment *appsv1.Deployment, rc *resources.ResourceContext) syncResult
}

type mysqlDeploymentController struct {
	client                kubernetes.Interface
	deploymentLister      appslisters.DeploymentLister
	mysqlServerDeployment *resources.MySQLServerDeployment
}

// createDeployment takes the representation of a deployment and creates it
func (mdc *mysqlDeploymentController) createDeployment(deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	return mdc.client.AppsV1().Deployments(deployment.Namespace).Create(
		context.TODO(), deployment, metav1.CreateOptions{})
}

// patchDeployment generates and applies the patch to the deployment
func (mdc *mysqlDeploymentController) patchDeployment(existingDeployment *appsv1.Deployment, updatedDeployment *appsv1.Deployment) (*appsv1.Deployment, error) {
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
	deployment, err := mdc.client.AppsV1().Deployments(existingDeployment.Namespace).Patch(
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
func (mdc *mysqlDeploymentController) ReconcileDeployment(
	ndb *v1alpha1.Ndb, deployment *appsv1.Deployment, rc *resources.ResourceContext) syncResult {

	// Nothing to reconcile if there is no existing deployment
	if deployment == nil {
		return finishProcessing()
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

	// There is a change in config generation. Patch deployment.
	// TODO : In case of scale down, patch the deployment before
	//        the config gets patched (if it gets patched). So
	//        that the mgmd doesn't end up with less number of
	//        API sections than the number of running MySQL Servers
	newDeployment := mdc.mysqlServerDeployment.NewDeployment(ndb, rc)
	updatedDeployment, err := mdc.patchDeployment(deployment, newDeployment)
	if err != nil {
		klog.Errorf("Failed to patch MySQL Server deployment")
		return errorWhileProcssing(err)
	}

	// If any change has been done to the spec of the deployment, exit and continue later
	if !deploymentComplete(updatedDeployment) {
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

	// Deployment doesn't exist - create it
	numberOfMySQLServers := *ndb.Spec.Mysqld.NodeCount
	klog.Infof("Creating a deployment of '%d' MySQL Servers", numberOfMySQLServers)
	deployment = mdc.mysqlServerDeployment.NewDeployment(ndb, rc)
	if _, err = mdc.createDeployment(deployment); err != nil {
		// Creating deployment failed
		klog.Errorf("Failed to create deployment of '%d' MySQL Servers with error: %s",
			numberOfMySQLServers, err)
		return nil, false, err
	}

	// New deployment was successfully created
	return deployment, false, nil
}
