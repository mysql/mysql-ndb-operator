// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package statefulset

import (
	"fmt"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// newService builds and returns a new Service for the nodes with the given nodeTypeSelector
func newService(ndb *v1.NdbCluster, ports []int32, nodeType string, headLess, loadBalancer bool) *corev1.Service {

	// Default Service Type is ClusterIP
	var clusterIP string
	serviceType := corev1.ServiceTypeClusterIP
	if loadBalancer {
		// create a LoadBalancer
		serviceType = corev1.ServiceTypeLoadBalancer
	} else if headLess {
		// create a headless service
		clusterIP = corev1.ClusterIPNone
	}

	serviceResourceLabel := nodeType + "-service"

	// Label for the Service Resource
	serviceLabel := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: serviceResourceLabel,
	})

	// Label Selector for the pods
	selectorLabel := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterNodeTypeLabel: nodeType,
	})

	var servicePorts []corev1.ServicePort
	for i, port := range ports {
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name: fmt.Sprintf("%s-port-%d", serviceResourceLabel, i),
			Port: port,
		})
	}

	// build a Service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          serviceLabel,
			Name:            ndb.GetServiceName(nodeType),
			Namespace:       ndb.GetNamespace(),
			OwnerReferences: ndb.GetOwnerReferences(),
		},
		Spec: corev1.ServiceSpec{
			PublishNotReadyAddresses: true,
			Ports:                    servicePorts,
			Selector:                 selectorLabel,
			ClusterIP:                clusterIP,
			Type:                     serviceType,
		},
	}

	return svc
}
