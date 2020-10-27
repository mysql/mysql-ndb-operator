// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"github.com/ocklin/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/ocklin/ndb-operator/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const ConfigMapName = "ndb-config-ini"

func GenerateConfigMapObject(ndb *v1alpha1.Ndb) *corev1.ConfigMap {

	/*
		kind: ConfigMap
		apiVersion: v1
		metadata:
			name: config-ini
			namespace: default
			#uid: a7ec90d9-f2a9-11ea-95f5-000d3a2ebd7f
			#resourceVersion: '58127766'
			#creationTimestamp: '2020-09-09T14:35:06Z'
		data:
			config.ini: |
				[DB DEFAULT]
				....
	*/

	configStr, err := GetConfigString(ndb)
	if err != nil {
		return nil
	}

	data := map[string]string{
		"config.ini": configStr,
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: ndb.Namespace,
			Labels: map[string]string{
				constants.ClusterLabel: ndb.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(ndb, schema.GroupVersionKind{
					Group:   corev1.SchemeGroupVersion.Group,
					Version: corev1.SchemeGroupVersion.Version,
					Kind:    "Ndb",
				}),
			},
		},
		Data: data,
	}

	return cm
}
