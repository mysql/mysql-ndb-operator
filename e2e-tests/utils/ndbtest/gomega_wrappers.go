// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbtest

import (
	"github.com/onsi/gomega"
)

func ExpectError(err error, optionalDescription ...interface{}) {
	gomega.ExpectWithOffset(1, err).Should(gomega.HaveOccurred(), optionalDescription...)
}

func ExpectNoError(err error, optionalDescription ...interface{}) {
	gomega.ExpectWithOffset(1, err).Should(gomega.Succeed(), optionalDescription...)
}
