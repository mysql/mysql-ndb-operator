// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"

	"github.com/mysql/ndb-operator/pkg/resources"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/klog/v2"
)

// StatefulSetControlInterface defines the interface that
// wraps around the create and update of StatefulSets for node types
type StatefulSetControlInterface interface {
	GetTypeName() string
	EnsureStatefulSet(ctx context.Context, sc *SyncContext) (*apps.StatefulSet, bool, error)
}

// ndbNodeStatefulSetImpl implements the StatefulSetControlInterface to manage MySQL Cluster data nodes
type ndbNodeStatefulSetImpl struct {
	client             kubernetes.Interface
	statefulSetLister  appslisters.StatefulSetLister
	ndbNodeStatefulset resources.StatefulSetInterface
}

// NewNdbNodesStatefulSetControlInterface creates a new ndbNodeStatefulSetImpl
func NewNdbNodesStatefulSetControlInterface(
	client kubernetes.Interface,
	statefulSetLister appslisters.StatefulSetLister,
	ndbNodeStatefulset resources.StatefulSetInterface) StatefulSetControlInterface {
	return &ndbNodeStatefulSetImpl{
		client:             client,
		statefulSetLister:  statefulSetLister,
		ndbNodeStatefulset: ndbNodeStatefulset,
	}
}

// GetTypeName returns the type of the statefulSetInterface
// being controlled by the StatefulSetControlInterface
func (ndbSfset *ndbNodeStatefulSetImpl) GetTypeName() string {
	return ndbSfset.ndbNodeStatefulset.GetTypeName()
}

// EnsureStatefulSet creates a StatefulSet for the MySQL Cluster nodes if one doesn't exist already.
func (ndbSfset *ndbNodeStatefulSetImpl) EnsureStatefulSet(
	ctx context.Context, sc *SyncContext) (sfset *apps.StatefulSet, existed bool, err error) {

	// Get the StatefulSet from cache using statefulSetLister
	nc := sc.ndb
	sfsetName := ndbSfset.ndbNodeStatefulset.GetName(nc)
	sfset, err = ndbSfset.statefulSetLister.StatefulSets(nc.Namespace).Get(sfsetName)
	if err == nil {
		// The statefulSet already exists. Verify that
		// this is owned by the right NdbCluster resource.
		if err = sc.isOwnedByNdbCluster(sfset); err != nil {
			// StatefulSet is not owned by the NdbCluster resource.
			return nil, false, err
		}

		// StatefulSet exists and is owned by the NdbCluster resource
		return sfset, true, nil
	}

	if !errors.IsNotFound(err) {
		// Error retrieving the StatefulSet
		klog.Errorf("Failed to retrieve the StatefulSet %q for NdbCluster %q : %s", sfsetName, nc.Name, err)
		return nil, false, err
	}

	// StatefulSet doesn't exist. Create it.
	sfset = ndbSfset.ndbNodeStatefulset.NewStatefulSet(sc.configSummary, nc)
	klog.Infof("Creating StatefulSet \"%s/%s\" of type %q with Replica = %d",
		nc.Namespace, sfsetName, ndbSfset.ndbNodeStatefulset.GetTypeName(), *sfset.Spec.Replicas)
	sfsetInterface := ndbSfset.client.AppsV1().StatefulSets(nc.Namespace)
	sfset, err = sfsetInterface.Create(ctx, sfset, metav1.CreateOptions{})

	if err != nil {
		if errors.IsAlreadyExists(err) {
			// The StatefulSet was created already but the cache
			// didn't have it yet. This also implies that the
			// statefulset is not ready yet. Return existed = false
			// to make the sync handler stop processing. The sync
			// will continue when the statefulset becomes ready.
			return nil, false, nil
		}

		// Unexpected error. Failed to create the resource.
		klog.Errorf("Failed to create StatefulSet \"%s/%s\" : %s", nc.Namespace, sfsetName, err)
		return nil, false, err
	}

	return sfset, false, nil
}
