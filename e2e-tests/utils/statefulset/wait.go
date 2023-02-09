// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package statefulset

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

// statefulSetComplete tests if a statefulset is up and running
func statefulSetComplete(sfset *appsv1.StatefulSet, newStatus *appsv1.StatefulSetStatus) bool {
	return newStatus.UpdatedReplicas == *(sfset.Spec.Replicas) &&
		newStatus.ReadyReplicas == *(sfset.Spec.Replicas) &&
		newStatus.Replicas == *(sfset.Spec.Replicas) &&
		newStatus.ObservedGeneration >= sfset.Generation
}

// WaitForStatefulSetComplete waits for a statefulset to complete.
// adopted from WaitForDeploymentComplete
func WaitForStatefulSetComplete(c clientset.Interface, namespace, name string) error {
	var (
		sfset  *appsv1.StatefulSet
		reason string
	)

	err := wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		var err error
		sfset, err = c.AppsV1().StatefulSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		// When the deployment status and its underlying resources reach the desired state, we're done
		if statefulSetComplete(sfset, &sfset.Status) {
			return true, nil
		}

		reason = fmt.Sprintf("statefulset %s complete status: Generation: %d, Replicas: %d\n%#v",
			name, sfset.Generation, *(sfset.Spec.Replicas), sfset.Status)
		klog.V(4).Infof(reason)

		return false, nil
	})

	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("%s", reason)
	}
	if err != nil {
		return fmt.Errorf("error waiting for statefulset %q status to match expectation: %v", name, err)
	}
	return nil
}

// WaitForStatefulSetToDisappear waits for a statefulset to go away after deletion.
// adopted from WaitForDeploymentComplete
func WaitForStatefulSetToDisappear(c clientset.Interface, namespace, name string) error {
	var (
		sfset  *appsv1.StatefulSet
		reason string
	)

	err := wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		var err error
		sfset, err = c.AppsV1().StatefulSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			klog.Info(err)
			return false, err
		}

		reason = fmt.Sprintf("statefulset %s disappear status: Generation: %d, Replicas: %d\n%#v",
			name, sfset.Generation, *(sfset.Spec.Replicas), sfset.Status)
		klog.V(4).Infof(reason)

		return false, nil
	})

	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("%s", reason)
	}
	if err != nil {
		return fmt.Errorf("error waiting for statefulset %q status to go away: %v", name, err)
	}
	return nil
}
