// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package statefulset

import (
	"github.com/mysql/ndb-operator/pkg/constants"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetPodAntiAffinityRules returns the PodAntiAffinity definition
// with the given mySQLClusterNodeTypes. The mySQLClusterNodeTypes
// should be in the order of most to least preferred node type for
// co-location. The defined PodAntiAffinity rules ensure that the
// scheduler will first attempt to schedule the new pod onto a K8s
// worker node where none of the given mySQLClusterNodeTypes are
// running. If no such worker node is available, then the new pod
// will be co-located with existing MySQL Cluster node pods based
// on the preference specified via mySQLClusterNodeTypes.
func GetPodAntiAffinityRules(mySQLClusterNodeTypes []constants.NdbNodeType) *corev1.PodAntiAffinity {
	if len(mySQLClusterNodeTypes) != 3 {
		panic("GetPodAntiAffinityRules can handle only 3 MySQL Cluster NodeTypes")
	}
	// Use weights based on node type preference. Lower weights for
	// node types with higher preference and vice versa as the
	// Anti-Affinity is being defined here.
	//
	// K8s calculates the weight of a worker node based on Anti
	// Affinities by adding -1 * Weight of the PodAffinityTerm for
	// every pod that matches the PodAffinityTerm and runs in that
	// worker node. The new Pod will be scheduled on the worker node
	// with the most weight.
	//
	// Example :
	// Given 3 NodeTypes A, B, C, where A is the most preferred and
	// C is the least, the weights 5, 25, 50 ensure that the worker
	// nodes have the following order of preference when a new pod
	// with the AntiAffinity rule defined by this function has to
	// be scheduled onto one of them :
	// Worker Node where
	// (total weight of that worker node in bracket)
	//      1. No MySQL Cluster Nodes running (0)
	//      2. Only NodeType A is running (-5)
	//      3. Only NodeType B is running (-25)
	//      4. NodeTypes A and B are running (-30)
	//      5. Only NodeType C is running (-50)
	//      6. NodeTypes A and C are running (-55)
	//      7. NodeTypes B and C are running (-75)
	//      8. All NodeTypes A, B, C are running (-80)
	weights := []int32{5, 25, 50}

	// Generate the WeightedPodAffinities to be set in the PodAntiAffinity
	var weightedPodAffinities []corev1.WeightedPodAffinityTerm
	for i, mysqlClusterNodeType := range mySQLClusterNodeTypes {
		weightedPodAffinities = append(weightedPodAffinities, corev1.WeightedPodAffinityTerm{
			Weight: weights[i],
			PodAffinityTerm: corev1.PodAffinityTerm{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						constants.ClusterNodeTypeLabel: mysqlClusterNodeType,
					},
				},
				TopologyKey: "kubernetes.io/hostname",
			},
		})
	}

	return &corev1.PodAntiAffinity{
		PreferredDuringSchedulingIgnoredDuringExecution: weightedPodAffinities,
	}
}
