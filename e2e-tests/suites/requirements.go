package e2e

import (
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"k8s.io/kubernetes/test/e2e/framework"

	yaml_utils "github.com/mysql/ndb-operator/e2e-tests/utils/yaml"
)

var _ = framework.KubeDescribe("[Feature:requirements]", func() {

	framework.KubeDescribe("Testing yaml file reading", func() {
		ginkgo.It("should read a yaml file", func() {
			justAnExample := yaml_utils.YamlFile("e2e-tests/_manifests", "busybox")

			if !(len(justAnExample) > 0) {
				framework.Fail("Yaml file is empty")
			}
		})
	})

	framework.KubeDescribe("Testing to replace a property", func() {

		ginkgo.It("should replace all occurences of namespace in yaml", func() {

			data := yaml_utils.YamlFile("e2e-tests/_manifests", "prep_test_suite")

			gomega.Expect(data).NotTo(gomega.HaveLen(0))
			gomega.Ω(data).ShouldNot(gomega.HaveLen(0))

			var err error
			data, err = yaml_utils.ReplaceAllProperties(data, "namespace", "newsupernamespace")
			framework.ExpectNoError(err)

			if !(len(data) > 0) {
				framework.Fail("Result is empty")
			}

			gomega.ContainSubstring(data, "newsupernamespace")
			gomega.Ω(data).ShouldNot(gomega.ContainSubstring(data, "replace-me"))
		})
	})
})
