// This is a generated file. Do not edit directly.

module github.com/mysql/ndb-operator

go 1.16

require (
	github.com/go-sql-driver/mysql v1.6.0
	github.com/onsi/ginkgo/v2 v2.1.1
	github.com/onsi/gomega v1.18.1
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.20.15
	k8s.io/apimachinery v0.20.15
	k8s.io/client-go v0.20.15
	k8s.io/code-generator v0.20.15
	k8s.io/klog/v2 v2.60.1
	sigs.k8s.io/controller-tools v0.5.0
	sigs.k8s.io/kind v0.12.0
)

replace (
	github.com/mysql/ndb-operator/e2e-tests => ./e2e-tests
	github.com/mysql/ndb-operator/pkg => ./pkg
)
