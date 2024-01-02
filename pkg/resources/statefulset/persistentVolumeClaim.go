// Copyright (c) 2021, 2024, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package statefulset

import (
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// newPVC returns a new PVC based on the given spec
func newPVC(ndb *v1.NdbCluster, pvcName string, pvcSpec *corev1.PersistentVolumeClaimSpec) *corev1.PersistentVolumeClaim {
	// Labels for the resource
	pvcLabels := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: pvcName + "-pvc",
	})

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: ndb.Namespace,
			Labels:    pvcLabels,
		},
		Spec: *pvcSpec,
	}

	return pvc
}
