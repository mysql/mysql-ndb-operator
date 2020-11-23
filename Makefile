# Copyright (c) 2020, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

VERSION ?= 1.0.0

ARCH     ?= $(shell go env GOARCH)
OS       ?= $(shell go env GOOS)
UNAME_S  := $(shell uname -s)
PKG      := github.com/mysql/ndb-operator/
CMD_DIRECTORIES := $(sort $(dir $(wildcard ./cmd/*/)))
COMMANDS := $(CMD_DIRECTORIES:./cmd/%/=%)

# SRCDIR points to the current mysql ndb source
SRCDIR=/Users/bo/prg/mysql

# OS base dir is the *build* directory of your current runtime platform 
# the one you run the operator from when running it *outside* kubernetes
OSBASEDIR=/Users/bo/prg/mysql-bld/trunk

# point BASEDIR to your mysql ndb *build* directory (not install)
# BASEDIR is the docker target platform build dir - i.e. and OL8 linux build
BASEDIR=/Users/bo/Downloads/mysql-cluster-commercial-8.0.23-linux-x86_64
RTDIR=${BASEDIR}/bin


BINDIR   :=bin/
SBINDIR  :=sbin/
LIBDIR   :=lib64/mysql/


ifeq ($(UNAME_S),Darwin)
ifeq ($(OS),linux)
	  # Cross-compiling from OSX to linux, go install puts the binaries in $GOPATH/bin/$GOOS_$GOARCH
    BINARIES := $(addprefix $(GOPATH)/bin/$(OS)_$(ARCH)/,$(COMMANDS))
else
	  # Compiling on darwin for linux, go install puts the binaries in $GOPATH/bin
    BINARIES := $(addprefix $(GOPATH)/bin/,$(COMMANDS))
endif
else
ifeq ($(UNAME_S),Linux)
	# Compiling on linux for linux, go install puts the binaries in $GOPATH/bin
    BINARIES := $(addprefix $(GOPATH)/bin/,$(COMMANDS))
else
	$(error "Unsupported OS: $(UNAME_S)")
endif
endif

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"
CRD_GENERATED_PATH := "helm/crds"

.PHONY: all
all: build

.PHONY: build
build: ndbinfo-bin
	@echo "Building: $(BINARIES)"
	@echo "arch:     $(ARCH)"
	@echo "os:       $(OS)"
	@echo "version:  $(VERSION)"
	@echo "pkg:      $(PKG)"
	@echo "bin:      $(BINARIES)"
	@touch pkg/version/version.go # Important. Work around for https://github.com/golang/go/issues/18369
	ARCH=$(ARCH) OS=$(OS) VERSION=$(VERSION) PKG=$(PKG) ./hack/build.sh
	mkdir -p ./bin/$(OS)_$(ARCH)/
	cp $(BINARIES) ./bin/$(OS)_$(ARCH)/

.PHONY: binaries
binaries:
	@echo $(BINARIES)

# install-minimal 
#   copies the needed ndb binaries 
#
# from a build (not install) directory 
# to this local folder 
# for going into a container
.PHONY: install-minimal
install-minimal:
	install -m 0750 -d bin/mysql/$(SBINDIR)
	install -m 0750 -d bin/mysql/$(BINDIR)
	install -m 0750 -d bin/mysql/$(LIBDIR)

	install -m 0755 $(RTDIR)/mysqld \
					$(RTDIR)/mysqladmin \
					$(RTDIR)/ndb_mgmd \
					$(RTDIR)/ndbmtd bin/mysql/$(SBINDIR)

	install -m 0755 $(RTDIR)/mysql \
					$(RTDIR)/ndb_mgm bin/mysql/$(BINDIR)

# just a convenience as I never remember which way around
.PHONY: docker-build
docker-build: build-docker

.PHONY: build-docker
build-docker: build-docker-cluster 
	# build-docker-agent

.PHONY: build-docker-cluster
build-docker-cluster: install-minimal
	@docker build \
	-t mysql-cluster:$(VERSION) \
	-f docker/Dockerfile .

.PHONY: build-docker-agent
build-docker-agent:
	@docker build \
	-t ndb-agent:$(VERSION) \
	-f docker/ndb-agent/Dockerfile .


.PHONY: version
version:
	@echo $(VERSION)

.PHONY: clean
clean:
	rm -rf .go bin

.PHONY: generate
generate:
	./hack/update-codegen.sh

run:
	bin/$(OS)_$(ARCH)/ndb-operator --kubeconfig=$(HOME)/.kube/config 

run-agent:
	MY_POD_NAMESPACE=default \
	MY_NDB_NAME=example-ndb \
	MY_POD_SERVERPORT=1186 \
	bin/$(OS)_$(ARCH)/ndb-agent --kubeconfig=$(HOME)/.kube/config

# Generate manifests (i.e.) CRD.
# creationTimestamp in the CRD is always generated as null
# https://github.com/kubernetes-sigs/controller-tools/issues/402
# remove it as a workaround
# TODO: Generate RBAC from here as well
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./..." output:crd:artifacts:config=$(CRD_GENERATED_PATH)
	sed -i.crd.bak "/\ \ creationTimestamp\:\ null/d" $(CRD_GENERATED_PATH)/* && rm $(CRD_GENERATED_PATH)/*.crd.bak

# check if there is a controller-gen available in
# the PATH or $GOBIN or else download
# and install one in $GOBIN
controller-gen:
ifneq (, $(wildcard $(GOBIN)/controller-gen))
	@echo "Found controller-gen in $(GOBIN)"
CONTROLLER_GEN=$(GOBIN)/controller-gen
else ifneq (, $(shell which controller-gen))
	@echo "Found controller-gen in path"
CONTROLLER_GEN=$(shell which controller-gen)
else
	@echo "Downloading and installing controller-gen..."
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
endif

NDBINFO_CPP_DIR=pkg/ndb/cpp
NDBINFO_BLD_DIR=lib/ndb/$(OS)_$(ARCH)

ndbinfo-bin:
	mkdir -p $(NDBINFO_BLD_DIR)
	cmake -S $(NDBINFO_CPP_DIR) -B $(NDBINFO_BLD_DIR) -DNDB_SOURCE_DIR=$(SRCDIR) -DNDB_BUILD_DIR=$(OSBASEDIR)  
	cd $(NDBINFO_BLD_DIR) ; make -f ./Makefile
	mv $(NDBINFO_BLD_DIR)/libndbinfo_native* $(NDBINFO_CPP_DIR)
