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
	"k8s.io/klog"

	e2edeployment "k8s.io/kubernetes/test/e2e/framework/deployment"

	deploymentutil "k8s.io/kubernetes/pkg/controller/deployment/util"
)

// CreateDeploymentFromSpec creates a deployment.
// copied from k8s.io/kubernetes@v1.18.2/test/e2e/framework/deployment/fixtures.go
// but using own deployment instead
func CreateDeploymentFromSpec(client clientset.Interface, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	deployment, err := client.AppsV1().Deployments(deployment.Namespace).Create(context.TODO(), deployment, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("deployment %q Create API error: %v", deployment.Name, err)
	}
	klog.V(1).Infof("Waiting deployment %q to complete", deployment.Name)
	err = e2edeployment.WaitForDeploymentComplete(client, deployment)
	if err != nil {
		return nil, fmt.Errorf("deployment %q failed to complete: %v", deployment.Name, err)
	}
	return deployment, nil
}

// WaitForDeploymentComplete waits for the deployment to complete.
// copied and modified from k8s.io/kubernetes@v1.18.2/test/e2e/framework/
func WaitForDeploymentComplete(c clientset.Interface, namespace, name string, pollInterval, pollTimeout time.Duration) error {
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
		if deploymentutil.DeploymentComplete(deployment, &deployment.Status) {
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
func WaitForDeploymentToDisappear(c clientset.Interface, namespace, name string, pollInterval, pollTimeout time.Duration) error {
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
