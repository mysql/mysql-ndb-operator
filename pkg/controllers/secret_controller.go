package controllers

import (
	"context"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/resources"
	v1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog"
)

type SecretControlInterface interface {
	EnsureSecret(ctx context.Context, ndb *v1alpha1.Ndb) (*v1.Secret, error)
}

// mysqlRootPasswordSecretInterface implements SecretControlInterface
type mysqlRootPasswordSecretInterface struct {
	secretInterface typedcorev1.SecretInterface
}

// NewMySQLRootPasswordSecretInterface creates and returns a new secretController
func NewMySQLRootPasswordSecretInterface(client kubernetes.Interface, namespace string) SecretControlInterface {
	return &mysqlRootPasswordSecretInterface{
		secretInterface: client.CoreV1().Secrets(namespace),
	}
}

// EnsureSecret checks if a secret with the given name exists
// and creates a new one if it doesn't exist already
func (mrpsi *mysqlRootPasswordSecretInterface) EnsureSecret(ctx context.Context, ndb *v1alpha1.Ndb) (*v1.Secret, error) {
	secretName, customSecret := resources.GetMySQLRootPasswordSecretName(ndb)

	// Check if the secret exists
	secret, err := mrpsi.secretInterface.Get(ctx, secretName, metav1.GetOptions{})
	if err == nil {
		// Secret exists
		return secret, nil
	}

	if !errors.IsNotFound(err) {
		// Error retrieving the secret
		klog.Errorf("Failed to retrieve secret %s : %v", secretName, err)
		return nil, err
	}

	// Secret not found
	if customSecret {
		// Secret specified in the spec doesn't exist
		klog.Errorf("MySQL root password Secret specified in the Ndb Spec doesn't exist : %v", err)
		return nil, err
	}

	// Secret not found and not a custom secret - create a new one
	secret = resources.NewMySQLRootPasswordSecret(ndb)
	if secret, err = mrpsi.secretInterface.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		klog.Errorf("Failed to create secret %s : %v", secretName, err)
	}

	return secret, err
}
