package service

import (
	"context"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

func GetExternalIP(
	clientset kubernetes.Interface, namespace string, serviceName string) string {
	svc, err := clientset.CoreV1().Services(namespace).Get(
		context.TODO(), serviceName, metav1.GetOptions{})
	framework.ExpectNoError(err)

	// ensure that external ips are available
	// a failure here probably means there is no tunnel to k8s
	gomega.Expect(svc.Spec.Ports).NotTo(gomega.BeEmpty())
	gomega.Expect(svc.Status.LoadBalancer.Ingress).NotTo(gomega.BeEmpty())
	gomega.Expect(svc.Status.LoadBalancer.Ingress[0].IP).NotTo(gomega.BeEmpty())

	return svc.Status.LoadBalancer.Ingress[0].IP
}
