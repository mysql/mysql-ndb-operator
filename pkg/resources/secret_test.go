// Copyright (c) 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	cryptorand "crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var errEntropyUnavailable = errors.New("entropy unavailable")

func TestGeneratedMySQLPasswordsUseRawURLBase64RandomBytes(t *testing.T) {
	secretFactories := []func(*testing.T) *corev1.Secret{
		requireGeneratedMySQLRootPasswordSecret,
		requireGeneratedMySQLRootPasswordSecret,
		requireGeneratedMySQLNDBOperatorPasswordSecret,
		requireGeneratedMySQLNDBOperatorPasswordSecret,
	}

	expectedLength := base64.RawURLEncoding.EncodedLen(generatedPasswordBytesLength)
	seenPasswords := make(map[string]struct{}, len(secretFactories))
	for _, factory := range secretFactories {
		secret := factory(t)
		passwordBytes, ok := secret.Data[corev1.BasicAuthPasswordKey]
		if !ok {
			t.Fatalf("Secret %q does not contain %q", secret.Name, corev1.BasicAuthPasswordKey)
		}

		password := string(passwordBytes)
		if password == "" {
			t.Fatalf("Secret %q has an empty generated password", secret.Name)
		}
		if len(password) != expectedLength {
			t.Fatalf("Secret %q password length is %d, but expected %d", secret.Name, len(password), expectedLength)
		}
		if strings.ContainsAny(password, "+/=") {
			t.Fatalf("Secret %q password contains non raw URL base64 characters: %q", secret.Name, password)
		}
		if _, err := base64.RawURLEncoding.DecodeString(password); err != nil {
			t.Fatalf("Secret %q password is not raw URL base64: %v", secret.Name, err)
		}
		if _, ok := seenPasswords[password]; ok {
			t.Fatalf("Generated duplicate password %q", password)
		}
		seenPasswords[password] = struct{}{}
	}
}

func TestGenerateRandomPasswordReturnsEntropyError(t *testing.T) {
	oldReader := cryptorand.Reader
	cryptorand.Reader = failingReader{}
	defer func() {
		cryptorand.Reader = oldReader
	}()

	if _, err := NewMySQLRootPasswordSecret(testutils.NewTestNdb(metav1.NamespaceDefault, "test", 2)); !errors.Is(err, errEntropyUnavailable) {
		t.Fatalf("Expected entropy error %q, got %v", errEntropyUnavailable, err)
	}
}

func requireGeneratedMySQLRootPasswordSecret(t *testing.T) *corev1.Secret {
	t.Helper()
	secret, err := NewMySQLRootPasswordSecret(testutils.NewTestNdb(metav1.NamespaceDefault, "test", 2))
	if err != nil {
		t.Fatalf("Failed to generate root password secret: %v", err)
	}
	return secret
}

func requireGeneratedMySQLNDBOperatorPasswordSecret(t *testing.T) *corev1.Secret {
	t.Helper()
	secret, err := NewMySQLNDBOperatorPasswordSecret(testutils.NewTestNdb(metav1.NamespaceDefault, "test", 2))
	if err != nil {
		t.Fatalf("Failed to generate operator password secret: %v", err)
	}
	return secret
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errEntropyUnavailable
}
