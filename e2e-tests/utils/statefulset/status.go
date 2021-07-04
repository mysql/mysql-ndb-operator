// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package statefulset

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
)

// ExpectHasLabel expects that the statefulset has the given label.
func ExpectHasLabel(c clientset.Interface, namespace, sfsetName string, labelKey string, labelValue string) {
	ginkgo.By("verifying the statefulset has the label " + labelKey + " " + labelValue)
	sfset, err := c.AppsV1().StatefulSets(namespace).Get(context.TODO(), sfsetName, metav1.GetOptions{})
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	gomega.ExpectWithOffset(1, sfset.Labels[labelKey]).To(gomega.Equal(labelValue))
}

// ExpectHasReplicas expects that the statefulset has the given number of replicas.
func ExpectHasReplicas(c clientset.Interface, namespace, sfsetName string, replicas int) {
	ginkgo.By(fmt.Sprintf("verifying the statefulset has %d number of replicas", replicas))
	sfset, err := c.AppsV1().StatefulSets(namespace).Get(context.TODO(), sfsetName, metav1.GetOptions{})

	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	gomega.ExpectWithOffset(1, int(sfset.Status.Replicas)).To(gomega.Equal(replicas))
}
