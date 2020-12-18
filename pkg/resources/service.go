// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewService builds and returns a new Service resource
func NewService(ndb *v1alpha1.Ndb, nodeTypeSelector string, externalIP bool) *corev1.Service {

	// default service details
	serviceName := ndb.GetServiceName(nodeTypeSelector)
	svcType := corev1.ServiceTypeClusterIP
	clusterIP := corev1.ClusterIPNone
	serviceResourceLabel := nodeTypeSelector + "-service"

	// if externalIP is true, create an external load balancer service
	if externalIP {
		serviceName += "-ext"
		svcType = corev1.ServiceTypeLoadBalancer
		clusterIP = ""
		serviceResourceLabel += "-ext"
	}

	// Label for the Service Resource
	serviceLabel := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: serviceResourceLabel,
	})

	// Label Selector for the pods
	selectorLabel := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterNodeTypeLabel: nodeTypeSelector,
	})

	// build a Service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          serviceLabel,
			Name:            serviceName,
			OwnerReferences: []metav1.OwnerReference{ndb.GetOwnerReference()},
		},
		Spec: corev1.ServiceSpec{
			PublishNotReadyAddresses: true,
			Ports: []corev1.ServicePort{
				// TODO: two ports in array didn't work, at least not exposing via minikube tunnel
				//corev1.ServicePort{Port: 8080, Name: "agent", Protocol: "TCP"},
				corev1.ServicePort{Port: 1186, Name: "ndb-node", Protocol: "TCP"},
			},
			Selector:  selectorLabel,
			ClusterIP: clusterIP,
			Type:      svcType,
		},
	}

	return svc
}
