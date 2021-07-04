// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbtest

import (
	"github.com/onsi/ginkgo"
)

// DescribeFeature is simple wrapper to the ginkgo Describe.
// It additionally adds feature tag to the description
func DescribeFeature(text string, body func()) bool {
	return ginkgo.Describe("[Feature : "+text+"]", body)
}
