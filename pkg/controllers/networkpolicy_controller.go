// Copyright (c) 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/mysql/ndb-operator/pkg/resources"

	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	typednetworkingv1 "k8s.io/client-go/kubernetes/typed/networking/v1"
	listersnetworkingv1 "k8s.io/client-go/listers/networking/v1"
	klog "k8s.io/klog/v2"
)

// NetworkPolicyControlInterface manages the lifecycle of the mgmd ingress boundary.
type NetworkPolicyControlInterface interface {
	EnsureManagementNetworkPolicy(
		ctx context.Context, sc *SyncContext,
	) (*networkingv1.NetworkPolicy, bool, error)
	PatchManagementNetworkPolicy(
		ctx context.Context, sc *SyncContext, currentNetworkPolicy *networkingv1.NetworkPolicy,
	) error
}

type networkPolicyControl struct {
	k8sClient           kubernetes.Interface
	networkPolicyLister listersnetworkingv1.NetworkPolicyLister
	operatorNamespace   string
}

func newNetworkPolicyControl(
	client kubernetes.Interface,
	networkPolicyLister listersnetworkingv1.NetworkPolicyLister,
	operatorNamespace string) NetworkPolicyControlInterface {
	return &networkPolicyControl{
		k8sClient:           client,
		networkPolicyLister: networkPolicyLister,
		operatorNamespace:   operatorNamespace,
	}
}

func (npc *networkPolicyControl) getNetworkPolicyInterface(namespace string) typednetworkingv1.NetworkPolicyInterface {
	return npc.k8sClient.NetworkingV1().NetworkPolicies(namespace)
}

func (npc *networkPolicyControl) validateOperatorNamespace(sc *SyncContext) error {
	// Fail closed instead of generating a policy that cannot identify the operator peer.
	if npc.operatorNamespace != "" {
		return nil
	}

	err := fmt.Errorf("cannot ensure management NetworkPolicy for NdbCluster %q: operator namespace is unknown",
		getNamespacedName(sc.ndb))
	klog.Error(err)
	return err
}

// EnsureManagementNetworkPolicy creates the policy if absent and verifies ownership.
// Drift repair is kept separate, matching the lifecycle of other managed resources.
func (npc *networkPolicyControl) EnsureManagementNetworkPolicy(
	ctx context.Context, sc *SyncContext,
) (*networkingv1.NetworkPolicy, bool, error) {
	if err := npc.validateOperatorNamespace(sc); err != nil {
		return nil, false, err
	}

	// Read from the informer cache on the normal reconciliation path.
	nc := sc.ndb
	networkPolicyName := nc.GetManagementNetworkPolicyName()
	currentNetworkPolicy, err := npc.networkPolicyLister.NetworkPolicies(nc.Namespace).Get(networkPolicyName)

	if err == nil {
		if err := sc.isOwnedByNdbCluster(currentNetworkPolicy); err != nil {
			return nil, false, err
		}
		return currentNetworkPolicy, true, nil
	}

	if !apierrors.IsNotFound(err) {
		// A lister failure is not absence and must not trigger a conflicting create.
		klog.Errorf("Failed to retrieve NetworkPolicy %q : %s", getNamespacedName2(nc.Namespace, networkPolicyName), err)
		return nil, false, err
	}

	// Create only after the cache confirms that the policy is absent.
	networkPolicy := resources.NewManagementNetworkPolicy(nc, npc.operatorNamespace)
	klog.Infof("Creating a NetworkPolicy %q for NdbCluster resource %q",
		getNamespacedName(networkPolicy), getNamespacedName(nc))
	networkPolicyInterface := npc.getNetworkPolicyInterface(nc.Namespace)
	createdNetworkPolicy, err := networkPolicyInterface.Create(ctx, networkPolicy, metav1.CreateOptions{})
	if err == nil {
		return createdNetworkPolicy, false, nil
	}
	if !apierrors.IsAlreadyExists(err) {
		// Propagate API failures so the workqueue retries reconciliation.
		klog.Errorf("Error creating NetworkPolicy %q : %s", getNamespacedName(networkPolicy), err)
		return nil, false, err
	}

	// The lister was stale. Retrieve and validate the object that won the
	// create race before allowing reconciliation to continue.
	currentNetworkPolicy, err = networkPolicyInterface.Get(ctx, networkPolicyName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to retrieve existing NetworkPolicy %q after create conflict: %s",
			getNamespacedName(networkPolicy), err)
		return nil, false, err
	}

	if err := sc.isOwnedByNdbCluster(currentNetworkPolicy); err != nil {
		return nil, false, err
	}
	return currentNetworkPolicy, true, nil
}

// PatchManagementNetworkPolicy repairs drift independently of NdbCluster generation.
// NetworkPolicy is a security boundary, so every reconciliation checks its desired state.
func (npc *networkPolicyControl) PatchManagementNetworkPolicy(
	ctx context.Context,
	sc *SyncContext,
	currentNetworkPolicy *networkingv1.NetworkPolicy,
) error {
	if err := npc.validateOperatorNamespace(sc); err != nil {
		return err
	}

	if currentNetworkPolicy == nil {
		return fmt.Errorf("cannot patch management NetworkPolicy for NdbCluster %q: policy is nil",
			getNamespacedName(sc.ndb))
	}

	// Unlike other patch controllers, this method receives the cached object from
	// Ensure, so revalidate both its identity and ownership before writing it.
	expectedName := sc.ndb.GetManagementNetworkPolicyName()
	if currentNetworkPolicy.Namespace != sc.ndb.Namespace || currentNetworkPolicy.Name != expectedName {
		return fmt.Errorf("cannot patch unexpected NetworkPolicy %q for NdbCluster %q",
			getNamespacedName(currentNetworkPolicy), getNamespacedName(sc.ndb))
	}
	if err := sc.isOwnedByNdbCluster(currentNetworkPolicy); err != nil {
		return err
	}

	updatedNetworkPolicy := resources.NewManagementNetworkPolicy(sc.ndb, npc.operatorNamespace)
	return npc.patchNetworkPolicyIfRequired(ctx, currentNetworkPolicy, updatedNetworkPolicy)
}

func (npc *networkPolicyControl) patchNetworkPolicyIfRequired(
	ctx context.Context,
	currentNetworkPolicy *networkingv1.NetworkPolicy,
	updatedNetworkPolicy *networkingv1.NetworkPolicy) error {

	if !networkPolicyNeedsPatch(currentNetworkPolicy, updatedNetworkPolicy) {
		return nil
	}

	// Guard drift repair against overwriting a concurrent policy update.
	patch, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels":          updatedNetworkPolicy.Labels,
			"ownerReferences": updatedNetworkPolicy.OwnerReferences,
			"resourceVersion": currentNetworkPolicy.ResourceVersion,
		},
		"spec": updatedNetworkPolicy.Spec,
	})
	if err != nil {
		klog.Errorf("Failed to encode NetworkPolicy patch: %v", err)
		return err
	}

	_, err = npc.getNetworkPolicyInterface(currentNetworkPolicy.Namespace).Patch(
		ctx, currentNetworkPolicy.Name, types.MergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		klog.Errorf("Failed to patch NetworkPolicy %q : %s", getNamespacedName(currentNetworkPolicy), err)
		return err
	}

	klog.Infof("NetworkPolicy %q has been patched successfully", getNamespacedName(currentNetworkPolicy))
	return nil
}

func networkPolicyNeedsPatch(currentNetworkPolicy, updatedNetworkPolicy *networkingv1.NetworkPolicy) bool {
	if !reflect.DeepEqual(currentNetworkPolicy.Spec, updatedNetworkPolicy.Spec) ||
		!reflect.DeepEqual(currentNetworkPolicy.OwnerReferences, updatedNetworkPolicy.OwnerReferences) {
		return true
	}

	// Enforce operator labels while preserving unrelated labels added by users or tools.
	for key, value := range updatedNetworkPolicy.Labels {
		if currentNetworkPolicy.Labels[key] != value {
			return true
		}
	}

	return false
}
