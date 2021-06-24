package service

import (
	"context"
	"github.com/mysql/ndb-operator/pkg/helpers"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

func GetServiceAddressAndPort(
	clientset kubernetes.Interface, namespace string, serviceName string) (string, int32) {
	svc, err := clientset.CoreV1().Services(namespace).Get(
		context.TODO(), serviceName, metav1.GetOptions{})
	framework.ExpectNoError(err)

	svcAddress, svcPort := helpers.GetServiceAddressAndPort(svc)
	gomega.Expect(svcAddress).NotTo(gomega.BeEmpty(), "service address should not be empty")
	gomega.Expect(svcPort).NotTo(gomega.BeZero(), "service port should not be empty")
	return svcAddress, svcPort
}
