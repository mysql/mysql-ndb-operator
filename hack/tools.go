// +build tools

// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// This package imports things required by build scripts, to force `go mod` to see them as dependencies
package tools

import (
  _ "k8s.io/code-generator"
  _ "sigs.k8s.io/controller-tools/cmd/controller-gen"
  _ "sigs.k8s.io/kind"
)
