// Copyright (c) 2021, 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	generatedPasswordBytesLength = 24
	mysqldRootPassword           = "mysqld-root-password"
	ndbOperatorPassword          = "ndb-operator-password"
)

// generateRandomPassword generates a cryptographically secure random password
// of length n.
func generateRandomPassword(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("failed to generate random password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewBasicAuthSecretWithRandomPassword creates and returns a new
// basic authentication secret with a random password
func newBasicAuthSecretWithRandomPassword(ndb *v1.NdbCluster,
	secretName string, secretLabelPrefix string) (*corev1.Secret, error) {
	// Generate a random password of predefined length
	password, err := generateRandomPassword(generatedPasswordBytesLength)
	if err != nil {
		return nil, err
	}
	// Labels to be applied to the secret
	secretLabels := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: secretLabelPrefix + "-secret",
	})
	// build Secret and return
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          secretLabels,
			Name:            secretName,
			Namespace:       ndb.GetNamespace(),
			OwnerReferences: ndb.GetOwnerReferences(),
		},
		Data: map[string][]byte{corev1.BasicAuthPasswordKey: []byte(password)},
		Type: corev1.SecretTypeBasicAuth,
	}, nil
}

// GetMySQLRootPasswordSecretName returns the name of the root password secret
// and a bool flag to specify if it is a custom secret created by the user
func GetMySQLRootPasswordSecretName(ndb *v1.NdbCluster) (secretName string, customSecret bool) {
	if ndb.Spec.MysqlNode.RootPasswordSecretName != "" {
		return ndb.Spec.MysqlNode.RootPasswordSecretName, true
	}
	return ndb.Name + "-" + mysqldRootPassword, false
}

// NewMySQLRootPasswordSecret creates and returns a new root password secret
func NewMySQLRootPasswordSecret(ndb *v1.NdbCluster) (*corev1.Secret, error) {
	secretName, _ := GetMySQLRootPasswordSecretName(ndb)
	return newBasicAuthSecretWithRandomPassword(ndb, secretName, mysqldRootPassword)
}

// GetMySQLNDBOperatorPasswordSecretName returns the name of the ndb operator password secret
func GetMySQLNDBOperatorPasswordSecretName(nc *v1.NdbCluster) (secretName string) {
	return nc.Name + "-" + ndbOperatorPassword
}

// NewMySQLNDBOperatorPasswordSecret creates and returns a new root password secret
func NewMySQLNDBOperatorPasswordSecret(nc *v1.NdbCluster) (*corev1.Secret, error) {
	secretName := GetMySQLNDBOperatorPasswordSecretName(nc)
	return newBasicAuthSecretWithRandomPassword(nc, secretName, ndbOperatorPassword)
}
