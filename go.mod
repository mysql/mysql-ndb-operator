// This is a generated file. Do not edit directly.

module github.com/mysql/ndb-operator

go 1.12

require (
	github.com/pkg/errors v0.9.1
	golang.org/dl v0.0.0-20201014211523-4da6277e5455 // indirect
	k8s.io/api v0.17.0
	k8s.io/apimachinery v0.17.0
	k8s.io/client-go v0.17.0
	k8s.io/code-generator v0.17.0
	k8s.io/klog v1.0.0
)

replace (
	github.com/mysql/ndb-operator/pkg => ./pkg
	golang.org/x/sys => golang.org/x/sys v0.0.0-20190813064441-fde4db37ae7a // pinned to release-branch.go1.13
	golang.org/x/tools => golang.org/x/tools v0.0.0-20190821162956-65e3620a7ae7 // pinned to release-branch.go1.13
	k8s.io/api => k8s.io/api v0.0.0-20191121015604-11707872ac1c
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20191121015412-41065c7a8c2a
	k8s.io/client-go => k8s.io/client-go v0.0.0-20191121015835-571c0ef67034
	k8s.io/code-generator => k8s.io/code-generator v0.0.0-20191121015212-c4c8f8345c7e
)
