# Copyright (c) 2020, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Configurable variables that can be set when building the operator :

# To enable compiling for a particular platform,
# set ARCH and OS when calling make
# By default, the operator is compiled for the linux_amd64 platforms
ARCH ?= amd64
OS   ?= linux

# BASEDIR should point to the docker target platform MySQL Cluster build
#  i.e. an OL8 MySQL Cluster build or install directory
BASEDIR ?=
# Optional additional tag to the custom cluster docker image being built
IMAGE_TAG ?=

# SRCDIR points to the current mysql ndb source
SRCDIR ?=

# End of configurable variables

.PHONY: all
all: build

# Generate clientset, informers, listers and deepcopy for Ndb resource
.PHONY: generate
generate:
	./hack/update-codegen.sh

# If there is any change in the Ndb api definition or the helm charts,
# generate the install artifact (and implicitly the Ndb CRD)
INSTALL_ARTIFACT=artifacts/install/ndb-operator.yaml
$(INSTALL_ARTIFACT): $(shell find helm) $(shell find pkg/apis/ndbcontroller)
	./hack/generate-manifests.sh

# User friendly target name for CRD and release artifact generation
.PHONY: manifests
manifests: $(INSTALL_ARTIFACT)

.PHONY: build
build: manifests
	ARCH=$(ARCH) OS=$(OS) ./hack/build.sh

.PHONY: run
run:
	bin/$(OS)_$(ARCH)/ndb-operator --kubeconfig=$(HOME)/.kube/config --scripts_dir=pkg/helpers/scripts

.PHONY: clean
clean:
	rm -rf bin

# docker command with DOCKER_BUILDKIT=1
DOCKER_CMD := DOCKER_BUILDKIT=1 docker

# Build NDB Operator container image
.PHONY: operator-image
operator-image: build
	$(DOCKER_CMD) build -t mysql/ndb-operator:latest -f docker/ndb-operator/Dockerfile .

# Build e2e-tests-tests image in docker
.PHONY: e2e-tests-image
e2e-tests-image:
	$(DOCKER_CMD) build -t e2e-tests -f docker/e2e-tests/Dockerfile .

# Build MySQL Cluster container image
.PHONY: ndb-container-image
ndb-container-image:
	@SRCDIR=$(SRCDIR) BASEDIR=$(BASEDIR) IMAGE_TAG=$(IMAGE_TAG) ./docker/mysql-cluster/build.sh

.PHONY: test
test:
	go test -v --count=1 ./e2e-tests/suites/...

.PHONY: unit-test
unit-test:
	go test -v --count=1 ./pkg/...

.PHONY: e2e
e2e:
	go run e2e-tests/run-e2e-test.go

.PHONY: e2e-kind
e2e-kind:
	go run e2e-tests/run-e2e-test.go --use-kind

fmt:
	go fmt ./pkg/...
	go fmt ./cmd/...
	go fmt ./e2e-tests/...

