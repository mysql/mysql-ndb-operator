package resources

import (
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NewPodDisruptionBudget creates a PodDisruptionBudget allowing maximum 1 data node to be unavailable
func NewPodDisruptionBudget(ndb *v1alpha1.Ndb, nodeTypeSelector string) *policyv1beta1.PodDisruptionBudget {

	// Labels for the resource
	pdbLabels := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: "pdb-" + nodeTypeSelector,
	})

	// Labels for selector
	selectorLabels := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterNodeTypeLabel: nodeTypeSelector,
	})

	minAvailable := intstr.FromInt(int(ndb.Spec.NodeCount - 1))
	pdb := &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:            ndb.GetPodDisruptionBudgetName(nodeTypeSelector),
			Namespace:       ndb.Namespace,
			Labels:          pdbLabels,
			OwnerReferences: ndb.GetOwnerReferences(),
		},
		Spec: policyv1beta1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			MinAvailable: &minAvailable,
		},
	}

	return pdb
}
