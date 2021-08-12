// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

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
	IsControlledBy(ctx context.Context, secretName string, ndb *v1alpha1.NdbCluster) bool
	Ensure(ctx context.Context, ndb *v1alpha1.NdbCluster) (*v1.Secret, error)
	Delete(ctx context.Context, namespace, secretName string) error
}

// secretDefaults implements the default methods and fields for all secret types
type secretDefaults struct {
	client kubernetes.Interface
}

// secretInterface returns a typed/core/v1.SecretInterface
func (sd *secretDefaults) secretInterface(namespace string) typedcorev1.SecretInterface {
	return sd.client.CoreV1().Secrets(namespace)
}

// IsControlledBy returns if the given secret is owned by the NdbCluster object
func (sd *secretDefaults) IsControlledBy(ctx context.Context, secretName string, nc *v1alpha1.NdbCluster) bool {
	secret, err := sd.secretInterface(nc.Namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to retrieve Secret %q : %s", secretName, err)
		return false
	}

	return metav1.IsControlledBy(secret, nc)
}

// Delete deletes the secret from the namespace
func (sd *secretDefaults) Delete(ctx context.Context, namespace, secretName string) error {
	return sd.secretInterface(namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
}

// mysqlRootPasswordSecrets implements SecretControlInterface and
// can handle a password required for the MySQL root account.
type mysqlRootPasswordSecrets struct {
	secretDefaults
}

// NewMySQLRootPasswordSecretInterface creates and returns a new SecretControlInterface
func NewMySQLRootPasswordSecretInterface(client kubernetes.Interface) SecretControlInterface {
	return &mysqlRootPasswordSecrets{
		secretDefaults{
			client: client,
		},
	}
}

// Ensure checks if a secret with the given name exists
// and creates a new one if it doesn't exist already
func (mrps *mysqlRootPasswordSecrets) Ensure(ctx context.Context, ndb *v1alpha1.NdbCluster) (*v1.Secret, error) {
	secretName, customSecret := resources.GetMySQLRootPasswordSecretName(ndb)

	// Check if the secret exists
	secret, err := mrps.secretInterface(ndb.Namespace).Get(ctx, secretName, metav1.GetOptions{})
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
	secret, err = mrps.secretInterface(ndb.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		klog.Errorf("Failed to create secret %s : %v", secretName, err)
	}

	return secret, err
}
