// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package crd

import (
	ndbv1alpha1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewTestNdbCrd creates a new Ndb object for testing
func NewTestNdbCrd(namespace string, name string, datanodes, replicas, mysqlnodes int32) *ndbv1alpha1.NdbCluster {
	return &ndbv1alpha1.NdbCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: ndbv1alpha1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ndbv1alpha1.NdbClusterSpec{
			NodeCount:       datanodes,
			RedundancyLevel: replicas,
			Mysqld: &ndbv1alpha1.NdbMysqldSpec{
				NodeCount: mysqlnodes,
			},
		},
	}
}
