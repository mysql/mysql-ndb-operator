// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package deployment

import (
	"context"
	"fmt"

	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
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

// ExpectHasReplicas expects that the deployment has the given number of replicas.
func ExpectHasReplicas(c clientset.Interface, namespace, deploymentName string, replicas int) {
	ginkgo.By(fmt.Sprintf("verifying the deployment has %d number of replicas", replicas))
	deployment, err := c.AppsV1().Deployments(namespace).Get(context.TODO(), deploymentName, metav1.GetOptions{})

	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	gomega.ExpectWithOffset(1, int(deployment.Status.Replicas)).To(gomega.Equal(replicas))
}

// ExpectToBeNil expects that the deployment to be empty.
func ExpectToBeNil(c clientset.Interface, namespace, deploymentName string) {
	ginkgo.By("verifying that the deployment doesn't exist")
	_, err := c.AppsV1().Deployments(namespace).Get(context.TODO(), deploymentName, metav1.GetOptions{})

	gomega.ExpectWithOffset(1, errors.IsNotFound(err)).To(gomega.BeTrue())
}
