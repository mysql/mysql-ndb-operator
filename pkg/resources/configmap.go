// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"github.com/ocklin/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

func GetConfigFromConfigMapObject(cm *corev1.ConfigMap) (string, error) {

	configStr := ""
	if cm != nil && cm.Data != nil {
		if len(cm.Data) > 0 {
			if val, ok := cm.Data["config.ini"]; ok {
				configStr = val
			}
		}
	}

	return configStr, nil
}

/*
  injects a newly generated config map into an existing config map object
	returns a pointer to the changed original
*/
func InjectUpdateToConfigMapObject(ndb *v1alpha1.Ndb, dest *corev1.ConfigMap) *corev1.ConfigMap {

	// get an updated config string ...
	configStr, err := GetConfigString(ndb)
	if err != nil {
		return nil
	}

	data := map[string]string{
		"config.ini": configStr,
	}

	// ... and copy it to the config map object
	dest.Data = data

	return dest
}

func GenerateConfigMapObject(ndb *v1alpha1.Ndb) *corev1.ConfigMap {

	/*
		kind: ConfigMap
		apiVersion: v1
		metadata:
			name: config-ini
			namespace: default
		data:
			config.ini: |
				[DB DEFAULT]
				....
	*/

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ndb.GetConfigMapName(),
			Namespace: ndb.Namespace,
			Labels:    ndb.GetLabels(),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(ndb, schema.GroupVersionKind{
					Group:   corev1.SchemeGroupVersion.Group,
					Version: corev1.SchemeGroupVersion.Version,
					Kind:    "Ndb",
				}),
			},
		},
		Data: nil,
	}

	return InjectUpdateToConfigMapObject(ndb, cm)
}
