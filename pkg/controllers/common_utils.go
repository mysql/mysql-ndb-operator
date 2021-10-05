// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	Separator = "/"
)

// getNamespacedName returns the name of the object
// along with the Namespace of form <namespace>/<name>.
func getNamespacedName(meta v1.Object) string {
	return meta.GetNamespace() + Separator + meta.GetName()
}

// getNdbClusterKey returns a key for the
// given NdbCluster of form <namespace>/<name>.
func getNdbClusterKey(nc *v1alpha1.NdbCluster) string {
	return getNamespacedName(nc)
}
