// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package k8sutils

import (
	"context"
	"github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetWorkerNodes(ctx context.Context, clientset kubernetes.Interface) []corev1.Node {
	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	gomega.ExpectWithOffset(1, err).ShouldNot(gomega.HaveOccurred())
	return nodeList.Items
}

func UpdateWorkerNode(ctx context.Context, clientset kubernetes.Interface, node *corev1.Node) {
	_, err := clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	gomega.ExpectWithOffset(1, err).ShouldNot(gomega.HaveOccurred())
}
