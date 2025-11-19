// Copyright (c) 2025, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"testing"

	discoveryfake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
)

// helper to configure the fake discovery on the fake clientset
func setDiscovery(t *testing.T,
	cs *fake.Clientset, resources []*metav1.APIResourceList, version *version.Info) *discoveryfake.FakeDiscovery {
	t.Helper()
	fd := cs.Discovery().(*discoveryfake.FakeDiscovery)
	fd.Resources = resources
	fd.FakedServerVersion = version
	return fd
}

func TestServerSupportsV1Policy_DiscoverySuccess_ReturnsTrue(t *testing.T) {
	cs := fake.NewSimpleClientset()
	setDiscovery(t, cs, []*metav1.APIResourceList{
		{
			GroupVersion: "policy/v1",
			APIResources: []metav1.APIResource{
				{Name: "poddisruptionbudgets"},
			},
		},
	}, &version.Info{Major: "1", Minor: "33"})

	if ok := ServerSupportsV1Policy(cs); !ok {
		t.Fatalf("expected true when discovery finds policy/v1 poddisruptionbudgets")
	}
}

func TestServerSupportsV1Policy_DiscoverySuccess_NoPDBResource_ReturnsFalse(t *testing.T) {
	cs := fake.NewSimpleClientset()
	setDiscovery(t, cs, []*metav1.APIResourceList{
		{
			GroupVersion: "policy/v1",
			APIResources: []metav1.APIResource{
				{Name: "someotherresource"},
			},
		},
	}, &version.Info{Major: "1", Minor: "33"})

	if ok := ServerSupportsV1Policy(cs); ok {
		t.Fatalf("expected false when policy/v1 exists but poddisruptionbudgets resource is missing")
	}
}

func TestServerSupportsV1Policy_DiscoveryFails_FallbackTrueFor123Plus(t *testing.T) {
	cs := fake.NewSimpleClientset()
	// No policy/v1 resource list present to force ServerResourcesForGroupVersion error
	setDiscovery(t, cs, []*metav1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments"},
			},
		},
	}, &version.Info{Major: "1", Minor: "23"})

	if ok := ServerSupportsV1Policy(cs); !ok {
		t.Fatalf("expected true on fallback for server version >= 1.23")
	}
}

func TestServerSupportsV1Policy_DiscoveryFails_FallbackFalseFor122(t *testing.T) {
	cs := fake.NewSimpleClientset()
	// No policy/v1 resource list present to force ServerResourcesForGroupVersion error
	setDiscovery(t, cs, []*metav1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments"},
			},
		},
	}, &version.Info{Major: "1", Minor: "22"})

	if ok := ServerSupportsV1Policy(cs); ok {
		t.Fatalf("expected false on fallback for server version < 1.23")
	}
}
