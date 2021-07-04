// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewService builds and returns a new headless or a load balancer Service for the nodes with the given nodeTypeSelector
func NewService(ndb *v1alpha1.Ndb, port int32, nodeTypeSelector string, createLoadBalancer bool) *corev1.Service {

	// default headless service details
	serviceName := ndb.GetServiceName(nodeTypeSelector)
	svcType := corev1.ServiceTypeClusterIP
	clusterIP := corev1.ClusterIPNone
	serviceResourceLabel := nodeTypeSelector + "-service"

	// if externalIP is true, create an external load balancer service
	if createLoadBalancer {
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
			OwnerReferences: ndb.GetOwnerReferences(),
		},
		Spec: corev1.ServiceSpec{
			PublishNotReadyAddresses: true,
			Ports: []corev1.ServicePort{
				{Port: port, Name: serviceResourceLabel + "-port", Protocol: "TCP"},
			},
			Selector:  selectorLabel,
			ClusterIP: clusterIP,
			Type:      svcType,
		},
	}

	return svc
}
