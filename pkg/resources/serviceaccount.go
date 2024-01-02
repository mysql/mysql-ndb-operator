// Copyright (c) 2024, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewServiceAccount creates a ServiceAccount
func NewServiceAccount(ndb *v1.NdbCluster) *corev1.ServiceAccount {

	// Labels for the resource
	saLabels := ndb.GetCompleteLabels(map[string]string{
		constants.ClusterResourceTypeLabel: "ndb-serciveaccount",
	})

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:            ndb.GetServiceAccountName(),
			Namespace:       ndb.Namespace,
			Labels:          saLabels,
			OwnerReferences: ndb.GetOwnerReferences(),
		},
	}

	return sa
}
