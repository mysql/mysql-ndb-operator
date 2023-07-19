// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"

	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/resources"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	klog "k8s.io/klog/v2"
)

type DefaultSecretControlInterface interface {
	IsControlledBy(ctx context.Context, secretName string, ndb *v1.NdbCluster) bool
	Delete(ctx context.Context, namespace, secretName string) error
	ExtractPassword(ctx context.Context, namespace, name string) (string, error)
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
func (sd *secretDefaults) IsControlledBy(ctx context.Context, secretName string, nc *v1.NdbCluster) bool {
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

// ExtractPassword extracts the password from the given secret
func (sd *secretDefaults) ExtractPassword(ctx context.Context, namespace, name string) (string, error) {
	// Check if the secret exists
	secret, err := sd.secretInterface(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		// Secret does not exist
		klog.Errorf("Failed to retrieve Secret %q : %s", name, err)
		return "", err
	}

	return string(secret.Data[corev1.BasicAuthPasswordKey]), nil
}

// NewTDESecretInterface creates and returns a new DefaultSecretControlInterface
func NewTDESecretInterface(client kubernetes.Interface) DefaultSecretControlInterface {
	return &secretDefaults{
		client: client,
	}
}

type MySQLUserPasswordSecretControlInterface interface {
	DefaultSecretControlInterface
	EnsureMySQLRootPassword(ctx context.Context, ndb *v1.NdbCluster) (*corev1.Secret, error)
	EnsureNDBOperatorPassword(ctx context.Context, nc *v1.NdbCluster) (*corev1.Secret, error)
}

// mysqlUserPasswordSecrets implements MySQLUserPasswordSecretControlInterface and
// can handle a password required for the MySQL user accounts.
type mysqlUserPasswordSecrets struct {
	secretDefaults
}

// NewMySQLUserPasswordSecretInterface creates and returns a new MySQLUserPasswordSecretControlInterface
func NewMySQLUserPasswordSecretInterface(client kubernetes.Interface) MySQLUserPasswordSecretControlInterface {
	return &mysqlUserPasswordSecrets{
		secretDefaults{
			client: client,
		},
	}
}

// EnsureMySQLRootPassword checks if the MySQL root user secret exists
// and creates a new one if it doesn't exist already
func (mups *mysqlUserPasswordSecrets) EnsureMySQLRootPassword(ctx context.Context, ndb *v1.NdbCluster) (*corev1.Secret, error) {
	// Check if the root secret exists
	secretName, customSecret := resources.GetMySQLRootPasswordSecretName(ndb)

	secret, err := mups.secretInterface(ndb.Namespace).Get(ctx, secretName, metav1.GetOptions{})
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
	secret, err = mups.secretInterface(ndb.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		klog.Errorf("Failed to create secret %s : %v", secretName, err)
	}

	return secret, err
}

// EnsureNDBOperatorPassword checks if the MySQL ndb-operator user secret exists
// and creates a new one if it doesn't exist already
func (mups *mysqlUserPasswordSecrets) EnsureNDBOperatorPassword(
	ctx context.Context, nc *v1.NdbCluster) (*corev1.Secret, error) {
	// Check if the ndb-operator secret exists
	secretName := resources.GetMySQLNDBOperatorPasswordSecretName(nc)

	secret, err := mups.secretInterface(nc.Namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err == nil {
		// Secret exists
		return secret, nil
	}

	if !errors.IsNotFound(err) {
		// Error retrieving the secret
		klog.Errorf("Failed to retrieve secret %s : %v", secretName, err)
		return nil, err
	}

	// Secret not found create a new one
	secret = resources.NewMySQLNDBOperatorPasswordSecret(nc)
	secret, err = mups.secretInterface(nc.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		klog.Errorf("Failed to create secret %s : %v", secretName, err)
	}

	klog.Errorf("successfully created secret %s", secretName)
	return secret, err
}
