// This is a generated file. Do not edit directly.

module github.com/mysql/ndb-operator

go 1.16

require (
	github.com/go-sql-driver/mysql v1.6.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.17.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.20.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	k8s.io/code-generator v0.20.2
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	sigs.k8s.io/controller-tools v0.4.1
	sigs.k8s.io/kind v0.11.1
)

replace (
	github.com/mysql/ndb-operator/e2e-tests => ./e2e-tests
	github.com/mysql/ndb-operator/pkg => ./pkg
)
