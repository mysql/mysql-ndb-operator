// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"encoding/json"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/ndbconfig"
	"github.com/mysql/ndb-operator/pkg/resources"

	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/klog/v2"
)

// StatefulSetControlInterface defines the interface that
// wraps around the create and update of StatefulSets for node types
type StatefulSetControlInterface interface {
	GetTypeName() string
	EnsureStatefulSet(ctx context.Context, sc *SyncContext) (*apps.StatefulSet, bool, error)
	ReconcileStatefulSet(
		ctx context.Context, sfset *apps.StatefulSet, cs *ndbconfig.ConfigSummary, nc *v1alpha1.NdbCluster) syncResult
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

// patchStatefulSet generates and applies the patch to the StatefulSet
func (ndbSfset *ndbNodeStatefulSetImpl) patchStatefulSet(ctx context.Context,
	existingStatefulSet *apps.StatefulSet, updatedStatefulSet *apps.StatefulSet) syncResult {
	// JSON encode both StatefulSets
	existingJSON, err := json.Marshal(existingStatefulSet)
	if err != nil {
		klog.Errorf("Failed to encode existing StatefulSet: %v", err)
		return errorWhileProcessing(err)
	}
	updatedJSON, err := json.Marshal(updatedStatefulSet)
	if err != nil {
		klog.Errorf("Failed to encode updated StatefulSet: %v", err)
		return errorWhileProcessing(err)
	}

	// Generate the patch to be applied
	patch, err := strategicpatch.CreateTwoWayMergePatch(existingJSON, updatedJSON, apps.StatefulSet{})
	if err != nil {
		klog.Errorf("Failed to generate the patch to be applied: %v", err)
		return errorWhileProcessing(err)
	}

	// klog.Infof("Patching deployments.\nExisting : %v\n. Modified : %v\nPatch : %v", string(existingJSON), string(updatedJSON), string(patch))

	// Patch the StatefulSet
	sfsetInterface := ndbSfset.client.AppsV1().StatefulSets(existingStatefulSet.Namespace)
	updatedStatefulSet, err = sfsetInterface.Patch(
		ctx, existingStatefulSet.Name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		klog.Errorf("Failed to apply the patch to the StatefulSet %q : %s", getNamespacedName(existingStatefulSet), err)
		return errorWhileProcessing(err)
	}

	// Successfully applied the patch - StatefulSet has been updated.
	klog.Infof("StatefulSet %q has been patched successfully", getNamespacedName(updatedStatefulSet))

	if updatedStatefulSet.Generation == existingStatefulSet.Generation {
		// No changes to StatefulSet spec. Only annotations/labels were updated.
		// Continue processing
		return continueProcessing()
	}

	// StatefulSet was patched successfully.
	//
	// For the management node StatefulSet, the controller will
	// handle rolling out the update to all the pods. Reconciliation
	// will continue once the update has been rolled out, so finish
	// processing.
	//
	// For the data node StatefulSet, the operator will handle
	// rolling out the update over multiple reconciliation loops.
	// The controller will update the status of the data node
	// StatefulSet and that will immediately trigger the next
	// reconciliation loop, so finish processing.
	return finishProcessing()
}

func (ndbSfset *ndbNodeStatefulSetImpl) ReconcileStatefulSet(
	ctx context.Context, sfset *apps.StatefulSet, cs *ndbconfig.ConfigSummary, nc *v1alpha1.NdbCluster) syncResult {

	if workloadHasConfigGeneration(sfset, cs.NdbClusterGeneration) {
		// StatefulSet upto date
		return continueProcessing()
	}

	// StatefulSet has to be patched
	newSfset := ndbSfset.ndbNodeStatefulset.NewStatefulSet(cs, nc)
	return ndbSfset.patchStatefulSet(ctx, sfset, newSfset)
}
