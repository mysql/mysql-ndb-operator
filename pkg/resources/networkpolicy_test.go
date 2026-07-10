// Copyright (c) 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"testing"

	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestNewManagementNetworkPolicy(t *testing.T) {
	clusterName := "test-network-policy"
	namespace := "test-network-policy-namespace"
	ndb := testutils.NewTestNdb(namespace, clusterName, 2)

	networkPolicy := NewManagementNetworkPolicy(ndb, "ndb-operator")

	// Verify identity and garbage-collection ownership of the managed policy.
	if networkPolicy.Name != ndb.Name+"-mgmd-network-policy" {
		t.Fatalf("Unexpected NetworkPolicy name %q", networkPolicy.Name)
	}
	if networkPolicy.Namespace != ndb.Namespace {
		t.Fatalf("Unexpected NetworkPolicy namespace %q", networkPolicy.Namespace)
	}
	if networkPolicy.Labels[constants.ClusterLabel] != ndb.Name {
		t.Fatalf("Missing NdbCluster label on NetworkPolicy")
	}
	if networkPolicy.Labels[constants.ClusterResourceTypeLabel] != "mgmd-network-policy" {
		t.Fatalf("Unexpected NetworkPolicy resource type label %q",
			networkPolicy.Labels[constants.ClusterResourceTypeLabel])
	}
	if len(networkPolicy.OwnerReferences) != 1 || networkPolicy.OwnerReferences[0].Name != ndb.Name {
		t.Fatalf("Unexpected owner references: %#v", networkPolicy.OwnerReferences)
	}

	// Verify the policy isolates only this cluster's management pods.
	podSelector := networkPolicy.Spec.PodSelector.MatchLabels
	if podSelector[constants.ClusterLabel] != ndb.Name ||
		podSelector[constants.ClusterNodeTypeLabel] != constants.NdbNodeTypeMgmd {
		t.Fatalf("Unexpected pod selector: %#v", podSelector)
	}

	if len(networkPolicy.Spec.PolicyTypes) != 1 ||
		networkPolicy.Spec.PolicyTypes[0] != networkingv1.PolicyTypeIngress {
		t.Fatalf("Unexpected policy types: %#v", networkPolicy.Spec.PolicyTypes)
	}

	if len(networkPolicy.Spec.Ingress) != 1 {
		t.Fatalf("Expected one ingress rule, got %d", len(networkPolicy.Spec.Ingress))
	}

	ingressRule := networkPolicy.Spec.Ingress[0]
	if len(ingressRule.Ports) != 1 {
		t.Fatalf("Expected one ingress port, got %d", len(ingressRule.Ports))
	}
	port := ingressRule.Ports[0]
	if port.Protocol == nil || *port.Protocol != corev1.ProtocolTCP {
		t.Fatalf("Unexpected protocol: %#v", port.Protocol)
	}
	if port.Port == nil || port.Port.Type != intstr.Int || port.Port.IntVal != 1186 {
		t.Fatalf("Unexpected port: %#v", port.Port)
	}

	// Verify the three intended trust paths: cluster nodes, explicit clients, and operator.
	if len(ingressRule.From) != 3 {
		t.Fatalf("Expected three allowed peers, got %d", len(ingressRule.From))
	}

	ndbClusterPeers := ingressRule.From[0]
	if ndbClusterPeers.PodSelector == nil ||
		ndbClusterPeers.PodSelector.MatchLabels[constants.ClusterLabel] != ndb.Name {
		t.Fatalf("Unexpected cluster peer: %#v", ndbClusterPeers)
	}
	if len(ndbClusterPeers.PodSelector.MatchExpressions) != 1 {
		t.Fatalf("Expected cluster peer to require a node type label: %#v", ndbClusterPeers)
	}
	nodeTypeRequirement := ndbClusterPeers.PodSelector.MatchExpressions[0]
	if nodeTypeRequirement.Key != constants.ClusterNodeTypeLabel ||
		nodeTypeRequirement.Operator != metav1.LabelSelectorOpIn ||
		len(nodeTypeRequirement.Values) != 3 {
		t.Fatalf("Unexpected cluster node type requirement: %#v", nodeTypeRequirement)
	}

	accessLabelPeer := ingressRule.From[1]
	if accessLabelPeer.PodSelector == nil ||
		accessLabelPeer.PodSelector.MatchLabels[constants.ManagementAccessLabel] != ndb.Name {
		t.Fatalf("Unexpected explicit access peer: %#v", accessLabelPeer)
	}

	operatorPeer := ingressRule.From[2]
	if operatorPeer.PodSelector == nil ||
		operatorPeer.PodSelector.MatchLabels[operatorAppLabelKey] != operatorAppLabelValue {
		t.Fatalf("Unexpected operator pod selector: %#v", operatorPeer)
	}
	if operatorPeer.NamespaceSelector == nil ||
		operatorPeer.NamespaceSelector.MatchLabels[namespaceNameLabel] != "ndb-operator" {
		t.Fatalf("Unexpected operator namespace selector: %#v", operatorPeer.NamespaceSelector)
	}
}

func TestNewManagementNetworkPolicyUsesSameNamespaceOperatorSelector(t *testing.T) {
	ndb := testutils.NewTestNdb("ndb-operator", "test", 2)
	networkPolicy := NewManagementNetworkPolicy(ndb, ndb.Namespace)
	operatorPeer := networkPolicy.Spec.Ingress[0].From[2]
	if operatorPeer.NamespaceSelector != nil {
		t.Fatalf("Same-namespace operator peer must not have a namespace selector: %#v",
			operatorPeer.NamespaceSelector)
	}
}
