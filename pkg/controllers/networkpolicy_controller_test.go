// Copyright (c) 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
	"github.com/mysql/ndb-operator/pkg/resources"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	listersnetworkingv1 "k8s.io/client-go/listers/networking/v1"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/events"
)

func newStaleNetworkPolicyControl(
	t *testing.T,
	existingNetworkPolicy *networkingv1.NetworkPolicy,
) (*networkPolicyControl, *SyncContext, *k8sfake.Clientset) {
	t.Helper()

	// Keep the API object out of the lister to reproduce a stale-cache create race.
	ndb := testutils.NewTestNdb(existingNetworkPolicy.GetNamespace(), "test", 2)
	ndb.UID = types.UID("test-ndb-uid")
	client := k8sfake.NewSimpleClientset(existingNetworkPolicy)
	lister := listersnetworkingv1.NewNetworkPolicyLister(cache.NewIndexer(
		cache.MetaNamespaceKeyFunc,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	))

	return &networkPolicyControl{
			k8sClient:           client,
			networkPolicyLister: lister,
			operatorNamespace:   ndb.Namespace,
		}, &SyncContext{
			ndb:              ndb,
			kubernetesClient: client,
			recorder:         events.NewFakeRecorder(1),
		}, client
}

func TestEnsureAndPatchManagementNetworkPolicyAfterAlreadyExists(t *testing.T) {
	ndb := testutils.NewTestNdb(metav1.NamespaceDefault, "test", 2)
	ndb.UID = types.UID("test-ndb-uid")
	existingNetworkPolicy := resources.NewManagementNetworkPolicy(ndb, ndb.Namespace)
	existingNetworkPolicy.ResourceVersion = "1"
	existingNetworkPolicy.Spec.Ingress = nil

	control, sc, client := newStaleNetworkPolicyControl(t, existingNetworkPolicy)
	client.PrependReactor("patch", "networkpolicies", func(action k8stesting.Action) (bool, runtime.Object, error) {
		// Drift repair must carry the version observed before the patch.
		patchAction := action.(k8stesting.PatchAction)
		var patch map[string]interface{}
		if err := json.Unmarshal(patchAction.GetPatch(), &patch); err != nil {
			t.Fatal("Failed to decode NetworkPolicy patch:", err)
		}
		metadata := patch["metadata"].(map[string]interface{})
		if metadata["resourceVersion"] != existingNetworkPolicy.ResourceVersion {
			t.Fatalf("NetworkPolicy patch does not guard resourceVersion: %#v", metadata)
		}
		return false, nil, nil
	})
	currentNetworkPolicy, existed, err := control.EnsureManagementNetworkPolicy(context.Background(), sc)
	if err != nil {
		t.Fatal("Unexpected error ensuring NetworkPolicy:", err)
	}
	if !existed {
		t.Fatal("Expected the API object found after AlreadyExists to be reported as existing")
	}
	if currentNetworkPolicy == nil {
		t.Fatal("Expected Ensure to return the API object found after AlreadyExists")
	}
	// Ensure only establishes existence and ownership; drift repair is explicit.
	for _, action := range client.Actions() {
		if action.GetVerb() == "patch" {
			t.Fatalf("Ensure unexpectedly patched the NetworkPolicy: %v", action)
		}
	}
	if err := control.PatchManagementNetworkPolicy(
		context.Background(), sc, currentNetworkPolicy); err != nil {
		t.Fatal("Unexpected error patching NetworkPolicy:", err)
	}

	actualNetworkPolicy, err := client.NetworkingV1().NetworkPolicies(ndb.Namespace).Get(
		context.Background(), ndb.GetManagementNetworkPolicyName(), metav1.GetOptions{})
	if err != nil {
		t.Fatal("Failed to retrieve reconciled NetworkPolicy:", err)
	}
	expectedNetworkPolicy := resources.NewManagementNetworkPolicy(ndb, ndb.Namespace)
	if !reflect.DeepEqual(actualNetworkPolicy.Spec, expectedNetworkPolicy.Spec) {
		t.Fatalf("NetworkPolicy was not reconciled after AlreadyExists\nExpected: %#v\nActual: %#v",
			expectedNetworkPolicy.Spec, actualNetworkPolicy.Spec)
	}
}

func TestEnsureManagementNetworkPolicyAlreadyExistsRejectsUnownedObject(t *testing.T) {
	ndb := testutils.NewTestNdb(metav1.NamespaceDefault, "test", 2)
	existingNetworkPolicy := resources.NewManagementNetworkPolicy(ndb, ndb.Namespace)
	existingNetworkPolicy.OwnerReferences = nil

	control, sc, _ := newStaleNetworkPolicyControl(t, existingNetworkPolicy)
	_, existed, err := control.EnsureManagementNetworkPolicy(context.Background(), sc)
	if err == nil {
		t.Fatal("Expected an ownership error for the existing NetworkPolicy")
	}
	if existed {
		t.Fatal("Unowned NetworkPolicy must not be reported as a managed existing resource")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Expected resource ownership error, got: %v", err)
	}
}

func TestEnsureManagementNetworkPolicyAlreadyExistsPropagatesGetError(t *testing.T) {
	ndb := testutils.NewTestNdb(metav1.NamespaceDefault, "test", 2)
	existingNetworkPolicy := resources.NewManagementNetworkPolicy(ndb, ndb.Namespace)

	control, sc, client := newStaleNetworkPolicyControl(t, existingNetworkPolicy)
	getError := errors.New("NetworkPolicy get failed")
	client.PrependReactor("get", "networkpolicies", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, getError
	})

	_, existed, err := control.EnsureManagementNetworkPolicy(context.Background(), sc)
	if !errors.Is(err, getError) {
		t.Fatalf("Expected get error, got: %v", err)
	}
	if existed {
		t.Fatal("NetworkPolicy must not be reported as existing when it cannot be retrieved")
	}
}

func TestEnsureManagementNetworkPolicyRejectsMissingOperatorNamespace(t *testing.T) {
	ndb := testutils.NewTestNdb(metav1.NamespaceDefault, "test", 2)
	client := k8sfake.NewSimpleClientset()
	lister := listersnetworkingv1.NewNetworkPolicyLister(cache.NewIndexer(
		cache.MetaNamespaceKeyFunc,
		cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
	))
	control := &networkPolicyControl{
		k8sClient:           client,
		networkPolicyLister: lister,
	}
	sc := &SyncContext{
		ndb:              ndb,
		kubernetesClient: client,
		recorder:         events.NewFakeRecorder(1),
	}

	_, existed, err := control.EnsureManagementNetworkPolicy(context.Background(), sc)
	if err == nil || !strings.Contains(err.Error(), "operator namespace") {
		t.Fatalf("Expected missing operator namespace error, got: %v", err)
	}
	if existed {
		t.Fatal("NetworkPolicy must not be reported as existing without an operator namespace")
	}
	if actions := client.Actions(); len(actions) != 0 {
		t.Fatalf("Expected no API actions without an operator namespace, got: %v", actions)
	}
}

func TestPatchManagementNetworkPolicyRejectsUnownedObject(t *testing.T) {
	ndb := testutils.NewTestNdb(metav1.NamespaceDefault, "test", 2)
	existingNetworkPolicy := resources.NewManagementNetworkPolicy(ndb, ndb.Namespace)
	existingNetworkPolicy.OwnerReferences = nil

	control, sc, client := newStaleNetworkPolicyControl(t, existingNetworkPolicy)
	err := control.PatchManagementNetworkPolicy(context.Background(), sc, existingNetworkPolicy)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Expected resource ownership error, got: %v", err)
	}
	if actions := client.Actions(); len(actions) != 0 {
		t.Fatalf("Unowned NetworkPolicy must not be patched, got: %v", actions)
	}
}

func TestPatchManagementNetworkPolicyRejectsUnexpectedObject(t *testing.T) {
	ndb := testutils.NewTestNdb(metav1.NamespaceDefault, "test", 2)
	existingNetworkPolicy := resources.NewManagementNetworkPolicy(ndb, ndb.Namespace)
	existingNetworkPolicy.Name += "-other"

	control, sc, client := newStaleNetworkPolicyControl(t, existingNetworkPolicy)
	err := control.PatchManagementNetworkPolicy(context.Background(), sc, existingNetworkPolicy)
	if err == nil || !strings.Contains(err.Error(), "unexpected NetworkPolicy") {
		t.Fatalf("Expected NetworkPolicy identity error, got: %v", err)
	}
	if actions := client.Actions(); len(actions) != 0 {
		t.Fatalf("Unexpected NetworkPolicy must not be patched, got: %v", actions)
	}
}
