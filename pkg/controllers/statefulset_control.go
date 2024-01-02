// Copyright (c) 2020, 2024, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"encoding/json"

	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/resources/statefulset"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	typedappsv1 "k8s.io/client-go/kubernetes/typed/apps/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	klog "k8s.io/klog/v2"
)

// ndbNodeStatefulSetImpl implements the NdbStatefulSetControlInterface to manage MySQL Cluster data nodes
type ndbNodeStatefulSetImpl struct {
	client             kubernetes.Interface
	statefulSetLister  appslisters.StatefulSetLister
	ndbNodeStatefulset statefulset.NdbStatefulSetInterface
}

// newMgmdStatefulSetController creates a new ndbNodeStatefulSetImpl for Management Nodes
func newMgmdStatefulSetController(
	client kubernetes.Interface,
	statefulSetLister appslisters.StatefulSetLister) *ndbNodeStatefulSetImpl {
	return &ndbNodeStatefulSetImpl{
		client:             client,
		statefulSetLister:  statefulSetLister,
		ndbNodeStatefulset: statefulset.NewMgmdStatefulSet(),
	}
}

// statefulSetInterface returns a typed/apps/v1.StatefulSetInterface
func (ndbSfset *ndbNodeStatefulSetImpl) statefulSetInterface(namespace string) typedappsv1.StatefulSetInterface {
	return ndbSfset.client.AppsV1().StatefulSets(namespace)
}

// GetTypeName returns the type of the statefulSetInterface
// being controlled by the NdbStatefulSetControlInterface
func (ndbSfset *ndbNodeStatefulSetImpl) GetTypeName() string {
	return ndbSfset.ndbNodeStatefulset.GetTypeName()
}

// GetStatefulSet retrieves the StatefulSet owned by the given NdbCluster resource
func (ndbSfset *ndbNodeStatefulSetImpl) GetStatefulSet(sc *SyncContext) (*appsv1.StatefulSet, error) {
	// Get the StatefulSet from cache using statefulSetLister
	nc := sc.ndb
	sfsetName := ndbSfset.ndbNodeStatefulset.GetName(nc)
	sfset, err := ndbSfset.statefulSetLister.StatefulSets(nc.Namespace).Get(sfsetName)

	if err != nil && !errors.IsNotFound(err) {
		klog.Errorf("Failed to retrieve the StatefulSet %q for NdbCluster %q : %s",
			sfsetName, getNamespacedName(nc), err)
		return nil, err
	}

	if errors.IsNotFound(err) {
		// StatefulSet doesn't exist yet
		return nil, nil
	}

	// StatefulSet exists. Verify ownership
	if err = sc.isOwnedByNdbCluster(sfset); err != nil {
		// StatefulSet is not owned by the current NdbCluster resource
		return nil, err
	}

	return sfset, nil
}

// createStatefulSet creates the statefulSet returned by statefulset.NdbStatefulSetInterface in K8s
func (ndbSfset *ndbNodeStatefulSetImpl) createStatefulSet(
	ctx context.Context, sc *SyncContext) (sfset *appsv1.StatefulSet, err error) {

	// Create the service to be used by the statefulset
	if _, err = sc.serviceController.ensureService(ctx, sc, ndbSfset.ndbNodeStatefulset); err != nil {
		return nil, err
	}

	// Create the service account to be used by the statefulset
	if _, err = sc.serviceaccountController.ensureServiceAccount(ctx, sc); err != nil {
		return nil, err
	}

	// Create StatefulSet
	nc := sc.ndb
	sfsetName := ndbSfset.ndbNodeStatefulset.GetName(nc)
	sfset, err = ndbSfset.ndbNodeStatefulset.NewStatefulSet(sc.configSummary, nc)
	if err != nil {
		return nil, err
	}
	klog.Infof("Creating StatefulSet %q of type %q with Replica = %d",
		getNamespacedName2(nc.Namespace, sfsetName), ndbSfset.ndbNodeStatefulset.GetTypeName(), *sfset.Spec.Replicas)
	sfsetInterface := ndbSfset.statefulSetInterface(nc.Namespace)
	sfset, err = sfsetInterface.Create(ctx, sfset, metav1.CreateOptions{})

	if err != nil {
		if errors.IsAlreadyExists(err) {
			// The StatefulSet was created already but the cache
			// didn't have it yet. This also implies that the
			// statefulset is not ready yet. Return err = nil
			// to make the sync handler stop processing. The sync
			// will continue when the statefulset becomes ready.
			return nil, nil
		}

		// Unexpected error. Failed to create the resource.
		klog.Errorf("Failed to create StatefulSet %q : %s", getNamespacedName2(nc.Namespace, sfsetName), err)
		return nil, err
	}

	// New StatefulSet was successfully created
	return sfset, nil
}

// deleteStatefulSet deletes the given statefulSet
func (ndbSfset *ndbNodeStatefulSetImpl) deleteStatefulSet(ctx context.Context, sfset *appsv1.StatefulSet, sc *SyncContext) error {
	err := ndbSfset.statefulSetInterface(sfset.Namespace).Delete(ctx, sfset.Name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		// Delete failed with an error.
		// Ignore NotFound error as this delete might be a redundant
		// step, caused by an outdated cache read.
		klog.Errorf("Failed to delete the StatefulSet %q : %s", getNamespacedName(sfset), err)
		return err
	}

	klog.Errorf("Deleted the StatefulSet %q", getNamespacedName(sfset))

	// Delete the governing Service
	return sc.serviceController.deleteService(ctx, sfset.Namespace, sc.ndb.GetServiceName(ndbSfset.GetTypeName()))
}

// EnsureStatefulSet creates a StatefulSet for the MySQL Cluster nodes if one doesn't exist already.
func (ndbSfset *ndbNodeStatefulSetImpl) EnsureStatefulSet(
	ctx context.Context, sc *SyncContext) (sfset *appsv1.StatefulSet, existed bool, err error) {

	if sfset, err = ndbSfset.GetStatefulSet(sc); err != nil {
		// Error retrieving sfset
		return nil, false, err
	} else if sfset != nil {
		// StatefulSet exists and is owned by the NdbCluster resource
		return sfset, true, nil
	}

	// StatefulSet doesn't exist. Create it.
	sfset, err = ndbSfset.createStatefulSet(ctx, sc)
	return sfset, false, err
}

// patchStatefulSet patches the statefulSet
func (ndbSfset *ndbNodeStatefulSetImpl) patchStatefulSet(ctx context.Context,
	existingStatefulSet *appsv1.StatefulSet, updatedStatefulSet *appsv1.StatefulSet) syncResult {

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
	patch, err := strategicpatch.CreateTwoWayMergePatch(existingJSON, updatedJSON, appsv1.StatefulSet{})
	if err != nil {
		klog.Errorf("Failed to generate the patch to be applied: %v", err)
		return errorWhileProcessing(err)
	}

	// klog.Infof("Patching StatefulSets.\nExisting : %v\n. Modified : %v\nPatch : %v", string(existingJSON), string(updatedJSON), string(patch))

	// Patch the StatefulSet
	sfsetInterface := ndbSfset.statefulSetInterface(existingStatefulSet.Namespace)
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
	ctx context.Context, sfset *appsv1.StatefulSet, sc *SyncContext) syncResult {

	cs := sc.configSummary

	if workloadHasConfigGeneration(sfset, cs.NdbClusterGeneration) {
		// Check if it is the second iteration of the sync handler. In the first iteration,
		// the --initial flag will be added to the data node pods, and the ConfigMap will
		// be patched to remove the DataNodeInitialRestart field to trigger the second iteration.
		// During this second iteration, the change will be noted by the statefulset, and it will
		// be patched again to remove the --initial flag from the data node pod's command argument.
		if !(ndbSfset.GetTypeName() == constants.NdbNodeTypeNdbmtd &&
			isInitialFlagSet(sfset) &&
			!sc.configSummary.DataNodeInitialRestart) {
			// StatefulSet upto date
			return continueProcessing()
		}
	}

	// StatefulSet has to be patched
	// Patch the Governing Service first
	if err := sc.serviceController.patchService(ctx, sc, ndbSfset.ndbNodeStatefulset); err != nil {
		return errorWhileProcessing(err)
	}

	// Patch the StatefulSet
	nc := sc.ndb
	updatedStatefulSet, err := ndbSfset.ndbNodeStatefulset.NewStatefulSet(cs, nc)
	if err != nil {
		return errorWhileProcessing(err)
	}

	if ndbSfset.GetTypeName() == constants.NdbNodeTypeNdbmtd &&
		*(sfset.Spec.Replicas) < *(updatedStatefulSet.Spec.Replicas) {
		// New data nodes are being added to MySQL Cluster
		// config but do not start the new nodes yet.
		*(updatedStatefulSet.Spec.Replicas) = *(sfset.Spec.Replicas)
		// Set the AddNodeOnlineInProgress Annotation
		updatedStatefulSet.Annotations[AddNodeOnlineInProgress] = "true"
	}

	return ndbSfset.patchStatefulSet(ctx, sfset, updatedStatefulSet)
}
