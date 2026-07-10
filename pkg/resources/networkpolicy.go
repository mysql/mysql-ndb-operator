// Copyright (c) 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	operatorAppLabelKey         = "app"
	operatorAppLabelValue       = "ndb-operator"
	namespaceNameLabel          = "kubernetes.io/metadata.name"
	managementServerPort  int32 = 1186
)

// NewManagementNetworkPolicy restricts mgmd to cluster components, explicitly
// authorized same-namespace pods, and the operator.
func NewManagementNetworkPolicy(ndb *v1.NdbCluster, operatorNamespace string) *networkingv1.NetworkPolicy {
	// Identify the policy as an owned NdbCluster resource.
	networkPolicyLabels := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: "mgmd-network-policy",
	})

	// Select only this cluster's management pods.
	mgmdPodSelector := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterNodeTypeLabel: constants.NdbNodeTypeMgmd,
	})

	// PodSelector-only peers are limited to the policy's namespace.
	ndbClusterPeers := []networkingv1.NetworkPolicyPeer{
		{
			PodSelector: &metav1.LabelSelector{
				MatchLabels: ndb.GetLabels(),
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      constants.ClusterNodeTypeLabel,
						Operator: metav1.LabelSelectorOpIn,
						Values: []string{
							constants.NdbNodeTypeMgmd,
							constants.NdbNodeTypeNdbmtd,
							constants.NdbNodeTypeMySQLD,
						},
					},
				},
			},
		},
		{
			PodSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					constants.ManagementAccessLabel: ndb.Name,
				},
			},
		},
	}

	operatorPeer := networkingv1.NetworkPolicyPeer{
		PodSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				operatorAppLabelKey: operatorAppLabelValue,
			},
		},
	}
	// Add a namespace selector only when the operator runs outside the cluster namespace.
	if operatorNamespace != ndb.Namespace {
		operatorPeer.NamespaceSelector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				namespaceNameLabel: operatorNamespace,
			},
		}
	}
	allowedPeers := append(ndbClusterPeers, operatorPeer)

	protocol := corev1.ProtocolTCP

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          networkPolicyLabels,
			Name:            ndb.GetManagementNetworkPolicyName(),
			Namespace:       ndb.Namespace,
			OwnerReferences: ndb.GetOwnerReferences(),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: mgmdPodSelector,
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: allowedPeers,
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &protocol,
							Port: &intstr.IntOrString{
								Type:   intstr.Int,
								IntVal: managementServerPort},
						},
					},
				},
			},
		},
	}
}
