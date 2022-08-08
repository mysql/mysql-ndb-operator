// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package statefulset

import (
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// newPVC returns a new PVC based on the given spec
func newPVC(ndb *v1alpha1.NdbCluster, pvcName string, pvcSpec *v1.PersistentVolumeClaimSpec) *v1.PersistentVolumeClaim {
	// Labels for the resource
	pvcLabels := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: pvcName + "-pvc",
	})

	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            pvcName,
			Namespace:       ndb.Namespace,
			Labels:          pvcLabels,
			OwnerReferences: ndb.GetOwnerReferences(),
		},
		Spec: *pvcSpec,
	}

	return pvc
}
