package e2e

import (
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"k8s.io/kubernetes/test/e2e/framework"
)

var _ = framework.KubeDescribe("[Feature:requirements]", func() {

	framework.KubeDescribe("Testing yaml file reading", func() {
		ginkgo.It("should read a yaml file", func() {
			justAnExample := YamlFile("artifacts/examples", "busybox")

			if !(len(justAnExample) > 0) {
				framework.Fail("Yaml file is empty")
			}
		})
	})

	framework.KubeDescribe("Testing to replace a property", func() {

		ginkgo.It("should replace all occurences of namespace in yaml", func() {

			data := YamlFile("e2e-tests/testfiles", "prep_test_suite")

			gomega.Expect(data).NotTo(gomega.HaveLen(0))
			gomega.Ω(data).ShouldNot(gomega.HaveLen(0))

			var err error
			data, err = ReplaceAllProperties(data, "namespace", "newsupernamespace")
			framework.ExpectNoError(err)

			if !(len(data) > 0) {
				framework.Fail("Result is empty")
			}

			gomega.ContainSubstring(data, "newsupernamespace")
			gomega.Ω(data).ShouldNot(gomega.ContainSubstring(data, "replace-me"))
		})
	})
})
