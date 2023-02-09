// Copyright (c) 2020, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NewPodDisruptionBudget creates a PodDisruptionBudget allowing maximum 1 data node to be unavailable
func NewPodDisruptionBudget(ndb *v1.NdbCluster, nodeTypeSelector string) *policyv1.PodDisruptionBudget {

	// Labels for the resource
	pdbLabels := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: "pdb-" + nodeTypeSelector,
	})

	// Labels for selector
	selectorLabels := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterNodeTypeLabel: nodeTypeSelector,
	})

	minAvailable := intstr.FromInt(int(ndb.Spec.DataNode.NodeCount - 1))
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:            ndb.GetPodDisruptionBudgetName(nodeTypeSelector),
			Namespace:       ndb.Namespace,
			Labels:          pdbLabels,
			OwnerReferences: ndb.GetOwnerReferences(),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			MinAvailable: &minAvailable,
		},
	}

	return pdb
}
