// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/ocklin/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/ocklin/ndb-operator/pkg/constants"
)

// NewForCluster will return a new headless Kubernetes service for a MySQL cluster
func NewService(ndb *v1alpha1.Ndb) *corev1.Service {
	mysqlPort := corev1.ServicePort{Port: 1186}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{constants.ClusterLabel: ndb.Name},
			Name:   ndb.Name,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(ndb, schema.GroupVersionKind{
					Group:   v1.SchemeGroupVersion.Group,
					Version: v1.SchemeGroupVersion.Version,
					Kind:    "Ndb",
				}),
			},
			Annotations: map[string]string{
				"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{mysqlPort},
			Selector: map[string]string{
				constants.ClusterLabel: ndb.Name,
			},
			ClusterIP: corev1.ClusterIPNone,
			Type:      corev1.ServiceTypeClusterIP,
			//Type: corev1.ServiceTypeNodePort,
		},
	}

	return svc
}
