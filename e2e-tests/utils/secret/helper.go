package secret

import (
	"context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	testRootPassword = "ndbpass"
)

func CreateSecretForMySQLRootAccount(clientset kubernetes.Interface, secretName, namespace string) {
	// build Secret, create it in K8s and return
	rootPassSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{v1.BasicAuthPasswordKey: []byte(testRootPassword)},
		Type: v1.SecretTypeBasicAuth,
	}

	_, err := clientset.CoreV1().Secrets(namespace).Create(context.TODO(), rootPassSecret, metav1.CreateOptions{})
	framework.ExpectNoError(err, "failed to create the custom secret")
}

func DeleteSecret(clientset kubernetes.Interface, secretName, namespace string) {
	err := clientset.CoreV1().Secrets(namespace).Delete(context.TODO(), secretName, metav1.DeleteOptions{})
	framework.ExpectNoError(err, "failed to delete the custom secret")
}
