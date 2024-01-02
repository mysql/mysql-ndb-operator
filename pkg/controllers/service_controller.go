// Copyright (c) 2022, 2024, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"fmt"

	"github.com/mysql/ndb-operator/pkg/resources/statefulset"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	klog "k8s.io/klog/v2"
)

type ServiceControlInterface interface {
	ensureService(
		ctx context.Context, sc *SyncContext, ndbSfset statefulset.NdbStatefulSetInterface) (*corev1.Service, error)
	patchService(
		ctx context.Context, sc *SyncContext, ndbSfset statefulset.NdbStatefulSetInterface) error
	deleteService(
		ctx context.Context, namespace, name string) error
}

type serviceControl struct {
	k8sClient     kubernetes.Interface
	serviceLister listerscorev1.ServiceLister
}

// NewServiceControl creates a new ServiceControl
func NewServiceControl(client kubernetes.Interface, serviceLister listerscorev1.ServiceLister) ServiceControlInterface {
	return &serviceControl{
		k8sClient:     client,
		serviceLister: serviceLister,
	}
}

// getServiceInterface retrieves the Service Interface from the API Server
func (svcCtrl *serviceControl) getServiceInterface(namespace string) typedcorev1.ServiceInterface {
	return svcCtrl.k8sClient.CoreV1().Services(namespace)
}

// EnsureService creates a service if it doesn't exist yet.
func (svcCtrl *serviceControl) ensureService(
	ctx context.Context, sc *SyncContext,
	ndbSfset statefulset.NdbStatefulSetInterface) (*corev1.Service, error) {

	nc := sc.ndb
	serviceName := nc.GetServiceName(ndbSfset.GetTypeName())

	svc, err := svcCtrl.serviceLister.Services(nc.Namespace).Get(serviceName)

	if err == nil {
		// Service exists already
		if err = sc.isOwnedByNdbCluster(svc); err != nil {
			// But it is not owned by the NdbCluster resource
			klog.Errorf(
				"Attempting to create service %q failed as it exists already but not owned by NdbCluster resource %q",
				serviceName, getNamespacedName(nc))
			return nil, err
		}

		// Service already exists and is owned by nc
		return svc, nil
	}

	if !apierrors.IsNotFound(err) {
		// Error other than NotFound
		klog.Errorf("Error getting Service %q from serviceLister : %s", serviceName, err)
		return nil, err
	}

	// Service not found - create it
	svc = ndbSfset.NewGoverningService(nc)
	klog.Infof("Creating a new Service %q for NdbCluster resource %q", getNamespacedName(svc), getNamespacedName(sc.ndb))
	svc, err = svcCtrl.getServiceInterface(sc.ndb.Namespace).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		// Create failed. Ignore AlreadyExists error as it
		// might have been caused due to an outdated cache read.
		klog.Errorf("Error creating Service %q : %s", getNamespacedName(svc), err)
		return nil, err
	}
	return svc, nil
}

// patchService patches the given service if required
func (svcCtrl *serviceControl) patchService(
	ctx context.Context, sc *SyncContext, ndbSfset statefulset.NdbStatefulSetInterface) error {
	// Get current service via ensureService.
	// Note : ensureService will also create the service if it is
	// missing (which is not possible unless it has been deleted manually).
	currentSvc, err := svcCtrl.ensureService(ctx, sc, ndbSfset)
	if err != nil {
		return err
	}

	// Get the updated service
	nc := sc.ndb
	updatedSvc := ndbSfset.NewGoverningService(nc)

	// Only changing the Service type is supported
	if currentSvc.Spec.Type == updatedSvc.Spec.Type {
		// No change to service
		return nil
	}

	// For some reason the "regular" patch method do not work for Services.
	// Use a JSON Merge patch instead
	jsonMergePatch := fmt.Sprintf(`{"spec":{"type":"%s"}}`, updatedSvc.Spec.Type)

	// Patch the service
	_, err = svcCtrl.getServiceInterface(currentSvc.Namespace).Patch(
		ctx, currentSvc.GetName(), types.MergePatchType, []byte(jsonMergePatch), metav1.PatchOptions{})
	if err != nil {
		klog.Errorf("Failed to patch the service %q : %s", getNamespacedName(currentSvc), err)
		return err
	}

	// Successfully applied the patch
	klog.Infof("Service %q has been patched successfully", getNamespacedName(currentSvc))
	return nil
}

// deleteService deletes the given service
func (svcCtrl *serviceControl) deleteService(
	ctx context.Context, namespace, name string) error {
	err := svcCtrl.getServiceInterface(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		// Delete failed with an error.
		// Ignore NotFound error as this delete might be a redundant
		// step, caused by an outdated cache read.
		klog.Errorf("Failed to delete the Service %q : %s", getNamespacedName2(namespace, name), err)
		return err
	}

	klog.Infof("Deleted Service %q", getNamespacedName2(namespace, name))
	return nil
}
