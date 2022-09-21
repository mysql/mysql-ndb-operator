//go:build tools
// +build tools

// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// This package imports additional libraries required by build scripts
// and test files, to force `go mod` to see them as dependencies.
package tools

import (
	_ "github.com/onsi/ginkgo/v2/ginkgo"
	_ "k8s.io/apimachinery/pkg/util/uuid"
	_ "k8s.io/code-generator"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
	_ "sigs.k8s.io/kind"
)
