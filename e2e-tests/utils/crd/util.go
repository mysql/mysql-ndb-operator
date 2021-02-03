package crd

import (
	"fmt"

	ndbv1alpha1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	ndbclientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
)

func int32Ptr(i int32) *int32 { return &i }

// NewTestNdbCrd creates a new Ndb object for testing
func NewTestNdbCrd(namespace string, name string, datanodes, replicas, mysqlnodes int) *ndbv1alpha1.Ndb {
	return &ndbv1alpha1.Ndb{
		TypeMeta: metav1.TypeMeta{
			APIVersion: ndbv1alpha1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ndbv1alpha1.NdbSpec{
			NodeCount:       int32Ptr(int32(datanodes)),
			RedundancyLevel: int32Ptr(int32(replicas)),
			Mysqld: ndbv1alpha1.NdbMysqldSpec{
				NodeCount: int32Ptr(int32(mysqlnodes)),
			},
		},
	}
}

func LoadClientset() (*ndbclientset.Clientset, error) {

	config, err := framework.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("error creating ndb client: %v", err.Error())
	}
	ndbc, err := ndbclientset.NewForConfig(config)
	return ndbc, nil
}
