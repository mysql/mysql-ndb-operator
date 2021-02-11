package helpers

import (
	ndbcontroller "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewTestNdb creates a new Ndb resource for testing
func NewTestNdb(namespace string, name string, noofnodes int) *ndbcontroller.Ndb {
	return &ndbcontroller.Ndb{
		TypeMeta: metav1.TypeMeta{APIVersion: ndbcontroller.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ndbcontroller.NdbSpec{
			NodeCount:       IntToInt32Ptr(int(noofnodes)),
			RedundancyLevel: IntToInt32Ptr(int(2)),
			Mysqld: &ndbcontroller.NdbMysqldSpec{
				NodeCount: int32(noofnodes),
			},
		},
	}
}
