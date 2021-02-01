# Copyright (c) 2020, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Configurable variables to be set when building the operator :

# Version of the ndb-operator being built
VERSION := 1.0.0

# To enable cross compiling, set ARCH and OS
# to the target OS before calling make
ARCH     ?= $(shell go env GOARCH)
OS       ?= $(shell go env GOOS)

# BASEDIR should point to the docker target platform MySQL Cluster build
#  i.e. an OL8 MySQL Cluster build or install directory
BASEDIR ?=
# String to be tagged to the custom cluster docker image being built
IMAGE_TAG ?=

# SRCDIR points to the current mysql ndb source
SRCDIR ?=

# OS base dir is the *build* directory of your current runtime platform 
# the one you run the operator from when running it *outside* kubernetes
OSBASEDIR ?=

# End of configurable variables

.PHONY: all
all: build

# Determine the go install location based on OS and ARCH
UNAME_S := $(shell uname -s)
PKG      := github.com/mysql/ndb-operator/
CMD_DIRECTORIES := $(sort $(dir $(wildcard ./cmd/*/)))
COMMANDS := $(CMD_DIRECTORIES:./cmd/%/=%)

.PHONY: build
build: 
	@echo "arch:     $(ARCH)"
	@echo "os:       $(OS)"
	@echo "version:  $(VERSION)"
	@echo "pkg:      $(PKG)"
	@touch pkg/version/version.go # Important. Work around for https://github.com/golang/go/issues/18369
	ARCH=$(ARCH) OS=$(OS) VERSION=$(VERSION) PKG=$(PKG) ./hack/build.sh

.PHONY: clean
clean:
	rm -rf .go bin

.PHONY: version
version:
	@echo $(VERSION)

# Build a MySQL Cluster container image
.PHONY: ndb-container-image
ndb-container-image:
	@BASEDIR=$(BASEDIR) IMAGE_TAG=$(IMAGE_TAG) ./hack/build-cluster-container-image.sh

# Build a MySQL Cluster container image
.PHONY: operator-image
operator-image: OS=linux
operator-image: build
	docker build -t ndb-operator:"${VERSION}" -f docker/ndb-operator/Dockerfile .

.PHONY: generate
generate:
	./hack/update-codegen.sh

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS=crd:trivialVersions=true
CRD_INPUT_PATH=./pkg/apis/...
CRD_GENERATED_PATH=helm/crds
CONTROLLER_GEN_CMD=go run ./vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go

# Generate manifests (i.e.) CRD.
# creationTimestamp in the CRD is always generated as null
# https://github.com/kubernetes-sigs/controller-tools/issues/402
# remove it as a workaround
.PHONY: manifests
manifests:
	$(CONTROLLER_GEN_CMD) $(CRD_OPTIONS) paths=$(CRD_INPUT_PATH) output:crd:artifacts:config=$(CRD_GENERATED_PATH)
	sed -i.crd.bak "/\ \ creationTimestamp\:\ null/d" $(CRD_GENERATED_PATH)/* && rm $(CRD_GENERATED_PATH)/*.crd.bak

.PHONY: run
run:
	bin/$(OS)_$(ARCH)/ndb-operator --kubeconfig=$(HOME)/.kube/config

NDBINFO_CPP_DIR=pkg/ndb/ndbinfo
NDBINFO_BLD_DIR=lib/ndb/$(OS)_$(ARCH)

ndbinfo-bin:
	rm -rf $(NDBINFO_BLD_DIR)
	mkdir -p $(NDBINFO_BLD_DIR)
	cmake -S $(NDBINFO_CPP_DIR) -B $(NDBINFO_BLD_DIR) -DNDB_SOURCE_DIR=$(SRCDIR) -DNDB_BUILD_DIR=$(OSBASEDIR)  
	cd $(NDBINFO_BLD_DIR) ; make -f ./Makefile
	mv $(NDBINFO_BLD_DIR)/libndbinfo_native* $(NDBINFO_CPP_DIR)

.PHONY: test
test:
	go test -v --count=1 e2e-tests/suites/**

.PHONY: e2e
e2e:
	go run e2e-tests/run-e2e-test.go

.PHONY: e2e-kind
e2e-kind:
	go run e2e-tests/run-e2e-test.go --use-kind

