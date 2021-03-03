package resources

import (
	"math/rand"
	"time"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var random = rand.New(rand.NewSource(time.Now().UnixNano()))

const (
	validPasswordChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	mysqldRootPassword = "mysqld-root-password"
)

// generateRandomPassword generates a random alpha numeric password of length n
func generateRandomPassword(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = validPasswordChars[random.Int63()%int64(len(validPasswordChars))]
	}
	return string(b)
}

// NewBasicAuthSecretWithRandomPassword creates and returns a new
// basic authentication secret with a random password
func newBasicAuthSecretWithRandomPassword(ndb *v1alpha1.Ndb,
	secretName string, secretLabelPrefix string) *v1.Secret {
	// Generate a random password of length 16
	rootPassword := generateRandomPassword(16)
	// Labels to be applied to the secret
	secretLabels := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: secretLabelPrefix + "-secret",
	})
	// build Secret and return
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          secretLabels,
			Name:            secretName,
			Namespace:       ndb.GetNamespace(),
			OwnerReferences: []metav1.OwnerReference{ndb.GetOwnerReference()},
		},
		Data: map[string][]byte{v1.BasicAuthPasswordKey: []byte(rootPassword)},
		Type: v1.SecretTypeBasicAuth,
	}
}

// GetMySQLRootPasswordSecretName returns the name of the root password secret
// and a bool flag to specify if it is a custom secret created by the user
func GetMySQLRootPasswordSecretName(ndb *v1alpha1.Ndb) (string, bool) {
	if ndb.Spec.Mysqld.RootPasswordSecretName != "" {
		return ndb.Spec.Mysqld.RootPasswordSecretName, true
	}
	return ndb.Name + "-" + mysqldRootPassword, false
}

// NewMySQLRootPasswordSecret creates and returns a new root password secret
func NewMySQLRootPasswordSecret(ndb *v1alpha1.Ndb) *v1.Secret {
	secretName, _ := GetMySQLRootPasswordSecretName(ndb)
	return newBasicAuthSecretWithRandomPassword(ndb, secretName, mysqldRootPassword)
}
