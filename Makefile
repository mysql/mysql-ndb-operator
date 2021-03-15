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
build: manifests
	@echo "Building ndb operator..."
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

# Build a ndb operator image in docker
.PHONY: operator-image
# this MUST be here to cross-build for Linux on non-Linux build hosts
operator-image: OS=linux
operator-image: build
	OS=linux docker build -t ndb-operator:"${VERSION}" -f docker/ndb-operator/Dockerfile .

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

.PHONY: run
run:
	bin/$(OS)_$(ARCH)/ndb-operator --kubeconfig=$(HOME)/.kube/config --scripts_dir=pkg/helpers/scripts

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

