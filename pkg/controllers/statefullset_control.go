// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/resources"
	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/klog"
)

/* StatefulSetControlInterface defines the interface that the
   wraps around the creation and update of StatefulSets for node types. It
   is implemented as an interface to enable testing. */

type StatefulSetControlInterface interface {
	GetTypeName() string
	EnsureStatefulSet(sc *SyncContext) (*apps.StatefulSet, bool, error)
	Patch(rc *resources.ResourceContext, ndb *v1alpha1.Ndb, old *apps.StatefulSet) (*apps.StatefulSet, error)
}

type realStatefulSetControl struct {
	client            kubernetes.Interface
	statefulSetLister appslisters.StatefulSetLister
	statefulSetType   resources.StatefulSetInterface
}

// NewRealStatefulSetControl creates a concrete implementation of the
// StatefulSetControlInterface.
func NewRealStatefulSetControl(client kubernetes.Interface, statefulSetLister appslisters.StatefulSetLister) StatefulSetControlInterface {
	return &realStatefulSetControl{client: client, statefulSetLister: statefulSetLister}
}

// GetTypeName returns the type of the statefulSetInterface
// being controlled by the StatefulSetControlInterface
func (rssc *realStatefulSetControl) GetTypeName() string {
	return rssc.statefulSetType.GetTypeName()
}

// PatchStatefulSet performs a direct patch update for the specified StatefulSet.
func patchStatefulSet(client kubernetes.Interface, oldData *apps.StatefulSet, newData *apps.StatefulSet) (*apps.StatefulSet, error) {
	originalJSON, err := json.Marshal(oldData)
	if err != nil {
		return nil, err
	}

	//klog.Infof("Patching StatefulSet old: %s", string(originalJSON))

	updatedJSON, err := json.Marshal(newData)
	if err != nil {
		return nil, err
	}

	//klog.Infof("Patching StatefulSet new: %s", string(updatedJSON))

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(
		originalJSON, updatedJSON, apps.StatefulSet{})
	if err != nil {
		return nil, err
	}
	klog.Infof("Patching StatefulSet %q: %s", types.NamespacedName{
		Namespace: oldData.Namespace,
		Name:      oldData.Name}, string(patchBytes))

	result, err := client.AppsV1().StatefulSets(oldData.Namespace).Patch(context.TODO(), oldData.Name,
		types.StrategicMergePatchType,
		patchBytes, metav1.PatchOptions{})

	if err != nil {
		klog.Errorf("Failed to patch StatefulSet: %v", err)
		return nil, err
	}

	return result, nil
}

func (rssc *realStatefulSetControl) Patch(rc *resources.ResourceContext, ndb *v1alpha1.Ndb, old *apps.StatefulSet) (*apps.StatefulSet, error) {

	oldCopy := old.DeepCopy()

	klog.Infof("Patch stateful set %s/%s Replicas: %d, DataNodes: %d",
		ndb.Namespace,
		rssc.statefulSetType.GetName(),
		ndb.Spec.RedundancyLevel,
		ndb.Spec.NodeCount)

	sfset := rssc.statefulSetType.NewStatefulSet(rc, ndb)

	return patchStatefulSet(rssc.client, oldCopy, sfset)
}

// EnsureStatefulSet creates a statefulset if there is none
// returns
//   the statefull set if created or already existing
//   true if it already existed
//   error is such occured
func (rssc *realStatefulSetControl) EnsureStatefulSet(sc *SyncContext) (*apps.StatefulSet, bool, error) {

	// Get the StatefulSet with the name specified in Ndb.spec
	sfset, err := rssc.statefulSetLister.StatefulSets(sc.ndb.Namespace).Get(rssc.statefulSetType.GetName())
	if err == nil {
		return sfset, true, nil
	}

	if !errors.IsNotFound(err) {
		return nil, false, err
	}

	rc := sc.resourceContext

	if !sc.resourceIsValid {
		klog.Infof("Creating stateful set %s/%s skipped due to invalid Ndb configuration",
			sc.ndb.Namespace, rssc.statefulSetType.GetName())
		// TODO error code?
		return nil, false, nil
	}

	// If the resource doesn't exist, we'll create it
	klog.Infof("Creating stateful set %s/%s Replicas: %d, Data Nodes: %d, Mgm Nodes: %d",
		sc.ndb.Namespace,
		rssc.statefulSetType.GetName(),
		rc.ReduncancyLevel,
		rc.ConfiguredNodeGroupCount*rc.ReduncancyLevel,
		rc.ManagementNodeCount)

	sfset = rssc.statefulSetType.NewStatefulSet(rc, sc.ndb)
	sfset, err = rssc.client.AppsV1().StatefulSets(sc.ndb.Namespace).Create(context.TODO(), sfset, metav1.CreateOptions{})

	if err != nil {
		// re-queue if something went wrong
		klog.Errorf("Failed to create stateful set %s/%s replicas: %d with error: %s",
			sc.ndb.Namespace, rssc.statefulSetType.GetName(),
			sc.ndb.Spec.NodeCount, err)

		return nil, false, err
	}

	return sfset, false, err
}
