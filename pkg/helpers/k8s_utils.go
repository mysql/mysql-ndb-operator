package helpers

import (
	corev1 "k8s.io/api/core/v1"
	"os"
)

// This file defines useful utility functions related to K8s

// IsAppRunningInsideK8s returns if the application is running inside a K8s pod or not
func IsAppRunningInsideK8s() bool {
	k8sHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	k8sPort := os.Getenv("KUBERNETES_SERVICE_PORT")

	if len(k8sHost) > 0 && len(k8sPort) > 0 {
		// Host and Port are set
		return true
	}

	return false
}

// GetServiceAddressAndPort returns the IP or the hostname through which the service is available
func GetServiceAddressAndPort(service *corev1.Service) (string, int32) {
	var servicePort int32
	if len(service.Spec.Ports) > 0 {
		servicePort = service.Spec.Ports[0].Port
	}

	switch service.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		if !IsAppRunningInsideK8s() {
			// The Application is running outside the K8s Cluster.
			// The service should be accessible via load balancer if the provider supports it.
			ingressPoints := service.Status.LoadBalancer.Ingress
			if len(ingressPoints) != 0 {
				ingressPoint := ingressPoints[0]
				if ingressPoint.IP != "" {
					return ingressPoint.IP, servicePort
				} else {
					return ingressPoint.Hostname, servicePort
				}
			} else {
				// ingress points not available
				return "", servicePort
			}
		}
		// The application is running inside the K8s Cluster - return ClusterIP
		fallthrough
	case corev1.ServiceTypeNodePort, corev1.ServiceTypeClusterIP:
		// The service should be accessible via ClusterIP.
		clusterIP := service.Spec.ClusterIP
		if clusterIP == corev1.ClusterIPNone {
			// this is a headless service
			return "", servicePort
		}
		return clusterIP, servicePort
	default:
		panic("service type not supported by GetServiceAddress yet")
	}
}
