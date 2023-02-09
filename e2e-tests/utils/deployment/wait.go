// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package deployment

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	klog "k8s.io/klog/v2"
)

const (
	// constant poll interval and timeout for waits
	pollInterval = 2 * time.Second
	pollTimeout  = 5 * time.Minute
)

// deploymentComplete considers a deployment to be complete
// once all of its desired replicas are updated and available,
// and no old pods are running.
func deploymentComplete(deployment *appsv1.Deployment) bool {
	return deployment.Status.UpdatedReplicas == *(deployment.Spec.Replicas) &&
		deployment.Status.Replicas == *(deployment.Spec.Replicas) &&
		deployment.Status.AvailableReplicas == *(deployment.Spec.Replicas) &&
		deployment.Status.ObservedGeneration >= deployment.Generation
}

// WaitForDeploymentComplete waits for the deployment to complete.
// copied and modified from k8s.io/kubernetes@v1.18.2/test/e2e/framework/
func WaitForDeploymentComplete(c clientset.Interface, namespace, name string) error {
	var (
		deployment *appsv1.Deployment
		reason     string
	)

	err := wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		var err error
		deployment, err = c.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}

		// When the deployment status and its underlying resources reach the desired state, we're done
		if deploymentComplete(deployment) {
			return true, nil
		}

		reason = fmt.Sprintf("deployment %s create status: Generation: %d, Replicas: %d\n%#v",
			name, deployment.Generation, *(deployment.Spec.Replicas), deployment.Status)
		klog.V(4).Infof(reason)

		return false, nil
	})

	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("%s", reason)
	}
	if err != nil {
		return fmt.Errorf("error waiting for deployment %q status to match expectation: %v", name, err)
	}
	return nil
}

// WaitForDeploymentToDisappear waits for the deployment to go away after deletion.
func WaitForDeploymentToDisappear(c clientset.Interface, namespace, name string) error {
	var (
		deployment *appsv1.Deployment
		reason     string
	)

	err := wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		var err error
		deployment, err = c.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}

		reason = fmt.Sprintf("deployment %s disappear status: Generation: %d, Replicas: %d\n%#v",
			name, deployment.Generation, *(deployment.Spec.Replicas), deployment.Status)
		klog.V(4).Infof(reason)

		return false, nil
	})

	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("%s", reason)
	}
	if err != nil {
		return fmt.Errorf("error waiting for deployment %q status to go away: %v", name, err)
	}
	return nil
}
