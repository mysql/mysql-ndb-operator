// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package util

import (
	yamlUtils "github.com/mysql/ndb-operator/e2e-tests/utils/yaml"
	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.Describe("Reading yaml from file", func() {
	var data string
	ginkgo.BeforeEach(func() {
		data = yamlUtils.YamlFile("e2e-tests/_manifests", "prep_test_suite")
	})

	ginkgo.Context("when the yaml is read successfully", func() {
		ginkgo.It("should return a non empty string", func() {
			gomega.Expect(data).NotTo(gomega.BeEmpty())
		})

	})

	ginkgo.Context("when a property is being replaced in the yaml object", func() {
		ginkgo.It("should succeed and replace all occurrences of namespace in yaml", func() {
			var err error
			data, err = yamlUtils.ReplaceAllProperties(data, "namespace", "newsupernamespace")

			ginkgo.By("not returning any error")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("returning a non empty string that has the yaml")
			gomega.Expect(data).NotTo(gomega.BeEmpty())

			ginkgo.By("not having old namespace in the returned string")
			gomega.Expect(data).NotTo(gomega.ContainSubstring("replace-me"))

			ginkgo.By("having new namespace in the returned string")
			gomega.Expect(data).To(gomega.ContainSubstring("newsupernamespace"))
		})
	})
})
