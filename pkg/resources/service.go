// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"fmt"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewService builds and returns a new Service for the nodes with the given nodeTypeSelector
func NewService(ndb *v1alpha1.NdbCluster, serviceType corev1.ServiceType, port int32, nodeTypeSelector string, createHeadLessService bool) *corev1.Service {
	var clusterIP string
	serviceName := ndb.GetServiceName(nodeTypeSelector)
	switch serviceType {
	case corev1.ServiceTypeClusterIP:
		if createHeadLessService {
			// Create a Headless service
			clusterIP = corev1.ClusterIPNone
		}
	case corev1.ServiceTypeLoadBalancer:
		if createHeadLessService {
			panic("load balancer service type can not be a headless service")
		}
	default:
		panic(fmt.Errorf("unsupported service type : %v", serviceType))
	}

	serviceResourceLabel := nodeTypeSelector + "-service"

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
			Type:      serviceType,
		},
	}

	return svc
}
