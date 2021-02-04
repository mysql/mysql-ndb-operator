// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/resources"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/klog"
)

type DeploymentControlInterface interface {
	EnsureDeployment(ndb *v1alpha1.Ndb) (*appsv1.Deployment, bool, error)
	ReconcileDeployment(ndb *v1alpha1.Ndb, deployment *appsv1.Deployment) syncResult
}

type mysqlDeploymentController struct {
	client                kubernetes.Interface
	deploymentLister      appslisters.DeploymentLister
	mysqlServerDeployment *resources.MySQLServerDeployment
}

func NewMySQLDeploymentController(client kubernetes.Interface, deploymentLister appslisters.DeploymentLister) *mysqlDeploymentController {
	return &mysqlDeploymentController{
		client:           client,
		deploymentLister: deploymentLister,
	}
}

// ReconcileDeployment compares the MySQL Server spec defined
// in Ndb resource and makes changes to the deployment if required
func (mdc *mysqlDeploymentController) ReconcileDeployment(ndb *v1alpha1.Ndb, deployment *appsv1.Deployment) syncResult {

	// Nothing to reconcile if there is no existing deployment
	if deployment == nil {
		// TODO interesting case - we should not never be here
		// if there is no deployment then it hasn't been created
		// should it be created even if replicas == 0 in MysqldSpec?
		return continueProcessing()
	}

	// If the number of replicas in Deployments is not equal to the spec nodecount, patch deployment
	// TODO: Compare all the other spec and config# here as well
	if *ndb.Spec.Mysqld.NodeCount != *deployment.Spec.Replicas {
		klog.Infof("Scaling MySQL Server. Old : %d, New : %d", *deployment.Spec.Replicas, *ndb.Spec.Mysqld.NodeCount)
		newDeployment := mdc.mysqlServerDeployment.NewDeployment(ndb)
		if _, err := mdc.client.AppsV1().Deployments(ndb.Namespace).Update(context.TODO(), newDeployment, metav1.UpdateOptions{}); err != nil {
			klog.Errorf("Failed to update MySQL Server deployment")
			return errorWhileProcssing(err)
		}
	}

	// success
	return continueProcessing()
}

// Check if the MySQLServerDeployment already exists. If not, create one.
func (mdc *mysqlDeploymentController) EnsureDeployment(ndb *v1alpha1.Ndb) (*appsv1.Deployment, bool, error) {

	// Get the deployment in the namespace of Ndb resource
	// with the name matching that of MySQLServerDeployment
	deployment, err := mdc.deploymentLister.Deployments(ndb.Namespace).Get(mdc.mysqlServerDeployment.GetName())

	// Return if the deployment exists already or
	// if listing failed due to some other error than not found
	if err == nil || !errors.IsNotFound(err) {
		return deployment, err == nil, err
	}

	// Deployment doesn't exist - create it
	numberOfMySQLServers := ndb.Spec.Mysqld.NodeCount
	klog.Infof("Creating a deployment of '%d' MySQL Servers", numberOfMySQLServers)
	deployment = mdc.mysqlServerDeployment.NewDeployment(ndb)
	if _, err = mdc.client.AppsV1().Deployments(ndb.Namespace).Create(context.TODO(), deployment, metav1.CreateOptions{}); err != nil {
		// Creating deployment failed
		klog.Errorf("Failed to create deployment of '%d' MySQL Servers with error: %s",
			numberOfMySQLServers, err)
		return nil, false, err
	}

	// New deployment was successfully created
	return deployment, false, nil
}
