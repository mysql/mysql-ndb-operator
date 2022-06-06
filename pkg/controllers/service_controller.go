// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"encoding/json"

	"github.com/mysql/ndb-operator/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
)

type ServiceControlInterface interface {
	EnsureService(sc *SyncContext, ctx context.Context, port int32,
		selector string, createLoadBalancer bool, createHeadLessService bool) (svc *corev1.Service, existed bool, err error)
	PatchService(svc *corev1.Service, upgradeToLoadBalancer bool) (*corev1.Service, error)
}

type serviceControl struct {
	k8sClient     kubernetes.Interface
	serviceLister corelisters.ServiceLister
}

// NewServiceControl creates a new ServiceControl
func NewServiceControl(client kubernetes.Interface, serviceLister corelisters.ServiceLister) ServiceControlInterface {
	return &serviceControl{
		k8sClient:     client,
		serviceLister: serviceLister,
	}
}

// getServiceInterface retrieves the Service Interface from the API Server
func (svcCtrl *serviceControl) getServiceInterface(namespace string) v1.ServiceInterface {
	return svcCtrl.k8sClient.CoreV1().Services(namespace)
}

// EnsureService creates a service if it doesn't exist yet. The function also patches the
// existing service if the user has made changes to the existing configuration
func (svcCtrl *serviceControl) EnsureService(
	sc *SyncContext, ctx context.Context, port int32,
	selector string, createLoadBalancer bool, createHeadLessService bool) (svc *corev1.Service, existed bool, err error) {

	serviceName := sc.ndb.GetServiceName(selector)
	var serviceType corev1.ServiceType

	if createLoadBalancer {
		serviceType = corev1.ServiceTypeLoadBalancer
	} else {
		serviceType = corev1.ServiceTypeClusterIP
	}

	svc, err = svcCtrl.serviceLister.Services(sc.ndb.Namespace).Get(serviceName)
	if err == nil {
		// Service exists already
		if err = sc.isOwnedByNdbCluster(svc); err != nil {
			// But it is not owned by the NdbCluster resource
			return nil, false, err
		}

		// Check if service type matches with the configuration and patch the
		// service if there are any changes.
		if svc.Spec.Type == serviceType {
			// Service exists and is owned by the current NdbCluster
			return svc, true, nil
		} else {
			// service needs to be patched
			// TODO: Check if the patching logic can be moved elsewhere
			newSvc, err := svcCtrl.PatchService(svc, createLoadBalancer)
			if err != nil {
				// error while patching the service
				return nil, false, err
			} else {
				return newSvc, true, nil
			}
		}
	}

	if !apierrors.IsNotFound(err) {
		return nil, false, err
	}

	// Service not found - create it
	klog.Infof("Creating a new Service %s for cluster %q", serviceName, getNamespacedName(sc.ndb))
	svc = resources.NewService(sc.ndb, serviceType, port, selector, createHeadLessService)
	svc, err = svcCtrl.getServiceInterface(sc.ndb.Namespace).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		// Create failed. Ignore AlreadyExists error as it might
		// have been caused due to a previous, outdated, cache read.
		return nil, false, err
	}
	return svc, false, err
}

// PatchService generates and applies the patch to the service
func (svcCtrl *serviceControl) PatchService(svc *corev1.Service, upgradeToLoadBalancer bool) (*corev1.Service, error) {
	updatedService := svc.DeepCopy()
	if upgradeToLoadBalancer {
		updatedService.Spec.Type = corev1.ServiceTypeLoadBalancer
	} else {
		updatedService.Spec.Type = corev1.ServiceTypeClusterIP
	}
	// JSON encode both services
	existingJSON, err := json.Marshal(svc)
	if err != nil {
		klog.Errorf("Failed to encode existing service: %v", err)
		return nil, err
	}
	updatedJSON, err := json.Marshal(updatedService)
	if err != nil {
		klog.Errorf("Failed to encode updated service: %v", err)
		return nil, err
	}

	// Generate the patch to be applied
	patch, err := strategicpatch.CreateTwoWayMergePatch(existingJSON, updatedJSON, corev1.Service{})
	if err != nil {
		klog.Errorf("Failed to generate the patch to be applied: %v", err)
		return nil, err
	}

	// Patch the service
	newSvc, err := svcCtrl.getServiceInterface(svc.Namespace).Patch(
		context.TODO(), svc.GetName(), types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	namespacedName := svc.GetNamespace() + "/" + svc.GetName()
	if err != nil {
		klog.Errorf("Failed to apply the patch to the service %q : %s", namespacedName, err)

		return nil, err
	}

	// successfully applied the patch
	klog.Infof("service %q has been patched successfully", namespacedName)
	return newSvc, nil
}
