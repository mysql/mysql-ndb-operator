package resources

import (
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// Create a PodDisruptionBudget
func NewPodDisruptionBudget(ndb *v1alpha1.Ndb) *policyv1beta1.PodDisruptionBudget {

	minAvailable := intstr.FromInt(int(*ndb.Spec.Ndbd.NodeCount - 1))
	labels := ndb.GetLabels()
	selectorLabels := ndb.GetDataNodeLabels()
	pdb := &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ndb.GetPodDisruptionBudgetName(),
			Namespace:   ndb.Namespace,
			Labels:      labels,
			Annotations: map[string]string{},
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
