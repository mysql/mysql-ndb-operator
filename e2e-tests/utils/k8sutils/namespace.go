// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package k8sutils

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func CreateNamespace(
	ctx context.Context, clientset kubernetes.Interface, namespace string) (*corev1.Namespace, error) {
	return clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}, metav1.CreateOptions{})
}

func DeleteNamespace(
	ctx context.Context, clientset kubernetes.Interface, namespace string) error {
	gracePeriodSeconds := int64(0)
	propagationPolicy := metav1.DeletePropagationForeground
	return clientset.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriodSeconds,
		PropagationPolicy:  &propagationPolicy,
	})
}
