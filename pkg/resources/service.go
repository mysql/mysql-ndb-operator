// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
)

// NewForCluster will return a new headless Kubernetes service for a MySQL cluster
func NewService(ndb *v1alpha1.Ndb, mgmd bool, externalIP bool, svcName string) *corev1.Service {

	selector := ndb.GetDataNodeLabels()
	svcType := corev1.ServiceTypeClusterIP
	clusterIP := corev1.ClusterIPNone

	if mgmd {
		selector = ndb.GetManagementNodeLabels()
	}

	if externalIP {
		svcType = corev1.ServiceTypeLoadBalancer
		clusterIP = ""
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          ndb.GetLabels(),
			Name:            svcName,
			OwnerReferences: []metav1.OwnerReference{ndb.GetOwnerReference()},
		},
		Spec: corev1.ServiceSpec{
			PublishNotReadyAddresses: true,
			Ports: []corev1.ServicePort{
				// TODO: two ports in array didn't work, at least not exposing via minikube tunnel
				//corev1.ServicePort{Port: 8080, Name: "agent", Protocol: "TCP"},
				corev1.ServicePort{Port: 1186, Name: "ndb-node", Protocol: "TCP"},
			},
			Selector:  selector,
			ClusterIP: clusterIP,
			Type:      svcType,
		},
	}

	return svc
}
