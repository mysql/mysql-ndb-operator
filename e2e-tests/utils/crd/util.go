// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package crd

import (
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewTestNdbCrd creates a new Ndb object for testing
func NewTestNdbCrd(namespace string, name string, datanodes, replicas, mysqlnodes int32) *v1.NdbCluster {
	return &v1.NdbCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.NdbClusterSpec{

			RedundancyLevel: replicas,
			ManagementNode:  &v1.NdbManagementNodeSpec{},
			DataNode: &v1.NdbDataNodeSpec{
				NodeCount: datanodes,
			},
			MysqlNode: &v1.NdbMysqldSpec{
				NodeCount: mysqlnodes,
			},
		},
	}
}
