# Copyright (c) 2020, 2021, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Configurable variables that can be set when building the operator :

# To enable compiling for a particular platform,
# set ARCH and OS when calling make
# By default, the operator is compiled for the linux_amd64 platforms
ARCH ?= amd64
OS   ?= linux

# Set this to 1 or ON to build operator in debug mode
WITH_DEBUG ?= OFF

# Git commit to be set in operator version
GIT_COMMIT_ID ?= $(shell git rev-parse --short HEAD)

# End of configurable variables

.PHONY: all
all: build

# Generate clientset, informers, listers and deepcopy for Ndb resource
.PHONY: generate
generate:
	./hack/update-codegen.sh

# If there is any change in the Ndb api definition or the helm charts,
# generate the install artifact (and implicitly the Ndb CRD)
INSTALL_ARTIFACT=deploy/manifests/ndb-operator.yaml
$(INSTALL_ARTIFACT): $(shell find deploy/charts/ndb-operator) $(shell find pkg/apis/ndbcontroller) hack/generate-manifests.sh
	./hack/generate-manifests.sh

# User friendly target name for CRD and release artifact generation
.PHONY: manifests
manifests: $(INSTALL_ARTIFACT)

.PHONY: build
build: manifests
	ARCH=$(ARCH) OS=$(OS) WITH_DEBUG=$(WITH_DEBUG) GIT_COMMIT_ID=$(GIT_COMMIT_ID) ./hack/build.sh

.PHONY: run
run: build
	bin/$(OS)_$(ARCH)/ndb-operator --kubeconfig=$(HOME)/.kube/config

.PHONY: clean
clean:
	rm -rf bin

# docker command with DOCKER_BUILDKIT=1
DOCKER_CMD := DOCKER_BUILDKIT=1 docker

# Build NDB Operator container image
.PHONY: operator-image
operator-image: build
	$(DOCKER_CMD) build -t localhost/mysql/ndb-operator:$(shell cat VERSION) -f docker/ndb-operator/Dockerfile .

# Build args to be passed to release docker build
BUILD_ARGS := --build-arg gitCommit=$(GIT_COMMIT_ID)
ifdef no_proxy
	BUILD_ARGS := $(BUILD_ARGS) --build-arg no_proxy=$(no_proxy)
endif
ifdef http_proxy
	BUILD_ARGS := $(BUILD_ARGS) --build-arg http_proxy=$(http_proxy)
endif
ifdef https_proxy
	BUILD_ARGS := $(BUILD_ARGS) --build-arg https_proxy=$(https_proxy)
endif
ifdef GOLANG_ALPINE_IMAGE
	BUILD_ARGS := $(BUILD_ARGS) --build-arg GOLANG_ALPINE_IMAGE=$(GOLANG_ALPINE_IMAGE)
endif

.PHONY: operator-image-release
operator-image-release:
	$(DOCKER_CMD) build -t mysql/ndb-operator:$(shell cat VERSION) -f docker/ndb-operator-release/Dockerfile $(BUILD_ARGS) .


# Build e2e-tests-tests image in docker
.PHONY: e2e-tests-image
ifdef GOLANG_IMAGE
e2e-tests-image:
	$(DOCKER_CMD) build -t e2e-tests -f docker/e2e-tests/Dockerfile --build-arg GOLANG_IMAGE=$(GOLANG_IMAGE) .
else
e2e-tests-image:
	$(DOCKER_CMD) build -t e2e-tests -f docker/e2e-tests/Dockerfile .
endif


.PHONY: unit-test
unit-test:
	go test -tags debug -v --count=1 ./pkg/...

# Run e2e tests against a local K8s Cluster
.PHONY: e2e
e2e:
	go run e2e-tests/run-e2e-test.go

.PHONY: e2e-kind
e2e-kind: operator-image
	go run e2e-tests/run-e2e-test.go -use-kind

# Run all unit tests and e2e test. Requires a minikube running
# with the tunnel open and the operator image to be available in it
.PHONY: test
test: unit-test e2e

fmt:
	go fmt ./pkg/...
	go fmt ./config/...
	go fmt ./cmd/...
	go fmt ./e2e-tests/...
	go fmt e2e-tests/run-e2e-test.go

