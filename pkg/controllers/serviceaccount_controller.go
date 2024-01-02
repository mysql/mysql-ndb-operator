// Copyright (c) 2024, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"

	"github.com/mysql/ndb-operator/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	klog "k8s.io/klog/v2"
)

type ServiceAccountControlInterface interface {
	ensureServiceAccount(
		ctx context.Context, sc *SyncContext) (*corev1.ServiceAccount, error)
	deleteServiceAccount(
		ctx context.Context, namespace, name string) error
}

type ServiceAccountControl struct {
	k8sClient            kubernetes.Interface
	ServiceAccountLister listerscorev1.ServiceAccountLister
}

// NewServiceAccountControl creates a new ServiceAccountControl
func NewServiceAccountControl(client kubernetes.Interface, ServiceAccountLister listerscorev1.ServiceAccountLister) ServiceAccountControlInterface {
	return &ServiceAccountControl{
		k8sClient:            client,
		ServiceAccountLister: ServiceAccountLister,
	}
}

// getServiceAccountInterface retrieves the ServiceAccount Interface from the API Server
func (saCtrl *ServiceAccountControl) getServiceAccountInterface(namespace string) typedcorev1.ServiceAccountInterface {
	return saCtrl.k8sClient.CoreV1().ServiceAccounts(namespace)
}

// EnsureServiceAccount creates a ServiceAccount if it doesn't exist yet.
func (saCtrl *ServiceAccountControl) ensureServiceAccount(
	ctx context.Context, sc *SyncContext) (*corev1.ServiceAccount, error) {

	nc := sc.ndb
	ServiceAccountName := nc.GetServiceAccountName()

	sa, err := saCtrl.ServiceAccountLister.ServiceAccounts(nc.Namespace).Get(ServiceAccountName)

	if err == nil {
		// ServiceAccount exists already
		if err = sc.isOwnedByNdbCluster(sa); err != nil {
			// But it is not owned by the NdbCluster resource
			klog.Errorf(
				"Attempting to create ServiceAccount %q failed as it exists already but not owned by NdbCluster resource %q",
				ServiceAccountName, getNamespacedName(nc))
			return nil, err
		}

		// ServiceAccount already exists and is owned by nc
		return sa, nil
	}

	if !apierrors.IsNotFound(err) {
		// Error other than NotFound
		klog.Errorf("Error getting ServiceAccount %q from ServiceAccountLister : %s", ServiceAccountName, err)
		return nil, err
	}

	// ServiceAccount not found - create it
	sa = resources.NewServiceAccount(nc)
	klog.Infof("Creating a new ServiceAccount %q for NdbCluster resource %q", getNamespacedName(sa), getNamespacedName(sc.ndb))
	sa, err = saCtrl.getServiceAccountInterface(sc.ndb.Namespace).Create(ctx, sa, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		// Create failed. Ignore AlreadyExists error as it
		// might have been caused due to an outdated cache read.
		klog.Errorf("Error creating ServiceAccount %q : %s", getNamespacedName(sa), err)
		return nil, err
	}
	return sa, nil
}

// deleteServiceAccount deletes the given ServiceAccount
func (saCtrl *ServiceAccountControl) deleteServiceAccount(
	ctx context.Context, namespace, name string) error {
	err := saCtrl.getServiceAccountInterface(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		// Delete failed with an error.
		// Ignore NotFound error as this delete might be a redundant
		// step, caused by an outdated cache read.
		klog.Errorf("Failed to delete the ServiceAccount %q : %s", getNamespacedName2(namespace, name), err)
		return err
	}

	klog.Infof("Deleted ServiceAccount %q", getNamespacedName2(namespace, name))
	return nil
}
