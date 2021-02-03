package deployment

import (
	"context"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

// ExpectHasLabel expects that the deployment has the given label.
func ExpectHasLabel(c clientset.Interface, namespace, deploymentName string, labelKey string, labelValue string) {
	ginkgo.By("verifying the deployment has the label " + labelKey + " " + labelValue)
	deployment, err := c.AppsV1().Deployments(namespace).Get(context.TODO(), deploymentName, metav1.GetOptions{})
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	gomega.ExpectWithOffset(1, deployment.Labels[labelKey]).To(gomega.Equal(labelValue))
}
