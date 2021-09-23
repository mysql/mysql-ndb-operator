// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	Separator = "/"
)

// getNamespacedName returns the name of the object
// along with the Namespace.
func getNamespacedName(meta v1.Object) string {
	return meta.GetName() + Separator + meta.GetNamespace()
}
