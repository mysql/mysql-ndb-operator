// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"testing"

	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
	"github.com/mysql/ndb-operator/pkg/resources"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMysqlRootPasswordSecrets(t *testing.T) {

	ns := metav1.NamespaceDefault
	ndb := testutils.NewTestNdb(ns, "test", 2)

	// Create fixture and start informers
	f := newFixture(t, ndb)
	defer f.close()
	f.start()

	sci := NewMySQLRootPasswordSecretInterface(f.kubeclient)

	// Test the secret control interface for default random password
	secret, err := sci.Ensure(context.TODO(), ndb)
	if err != nil {
		t.Errorf("Error ensuring secret : %v", err)
	}
	if secret == nil {
		t.Error("Error ensuring secret : secret is nil")
	}

	// expect one create action
	f.expectCreateAction(ns, "", "v1", "secrets", secret)

	// Test custom secret ensuring when the secret doesn't exist
	customSecretName := "custom-mysqld-root-password"
	ndb.Spec.Mysqld.RootPasswordSecretName = customSecretName
	// Ensuring should fail
	_, err = sci.Ensure(context.TODO(), ndb)
	if err == nil {
		t.Errorf("Expected '%s' secret not found error but got no error", customSecretName)
	} else if !errors.IsNotFound(err) {
		t.Errorf("Expected '%s' secret not found error but got : %v", customSecretName, err)
	}
	// No action is expected

	// Test custom secret ensuring when the secret exists
	// Create the custom secret
	customSecret := resources.NewMySQLRootPasswordSecret(ndb)
	customSecret.Name = customSecretName
	secret, err = f.kubeclient.CoreV1().Secrets(ns).Create(context.TODO(), customSecret, metav1.CreateOptions{})
	if err != nil {
		t.Errorf("Error creating custom secret : %v", err)
	}
	f.expectCreateAction(ns, "core", "v1", "secrets", secret)

	// Now ensuring should pass
	secret, err = sci.Ensure(context.TODO(), ndb)
	if err != nil {
		t.Errorf("Error ensuring custom secret '%s' : %v", customSecretName, err)
	}
	if secret == nil {
		t.Error("Error ensuring custom secret : secret is nil")
	}
	// No action is expected

	// Delete it and expect a delete action
	err = sci.Delete(context.Background(), ns, secret.Name)
	if err != nil {
		t.Errorf("Error deleting secret %q : %s", secret.Name, err)
	}
	f.expectDeleteAction(ns, "core", "v1", "secrets", secret.Name)

	// Validate all the actions
	f.checkActions()
}
