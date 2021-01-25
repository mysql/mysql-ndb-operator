#!/usr/bin/env bash

# Copyright (c) 2020, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Script to run an End to End test using kubetest2 and ginkgo

# Check if docker is running - needed for KinD
if ! docker info &> /dev/null; then
  echo >&2 "ERROR : The docker daemon is not running. Docker is required to run Kubetest2/KinD."
  exit 1
fi

# Deduce $GOBIN
_GOBIN=$(go env GOBIN)
if [ -z ${_GOBIN} ]; then
  # GOBIN is empty set it to $GOPATH/bin
  _GOBIN="$(go env GOPATH)/bin"
fi
echo "Using '${_GOBIN}' as \$GOBIN"

# go get wrapper for the binary installations
# uses cd and GO111MODULE=on to avoid go.mod getting updated
go_install () {
  (cd && GO111MODULE=on go get "$1")
}

# Verify required binaries exist or else, install them
if ! test -f "${_GOBIN}/kubetest2" ||
   ! test -f "${_GOBIN}/kubetest2-kind" ||
   ! test -f "${_GOBIN}/kubetest2-tester-exec"; then
  # Either kubetest, kind deployer or exec tester is missing get them all
  # Note : kubetest doesn't have release tags - so a known working version is pulled
  echo "Installing kubetest2 and friends at ${_GOBIN}..."
  go_install sigs.k8s.io/kubetest2@43699a7ba2
  go_install sigs.k8s.io/kubetest2/kubetest2-kind@43699a7ba2
  go_install sigs.k8s.io/kubetest2/kubetest2-tester-exec@43699a7ba2
fi

PATH="${PATH}:${_GOBIN}"

if ! kind --version &> /dev/null; then
  # No kind in path
  # Install it in GOBIN
  echo "Installing kind at ${_GOBIN}..."
  go_install sigs.k8s.io/kind@v0.9.0
fi

# Prepare arguments for kubetest2
KUBETEST2=(kubetest2 kind --cluster-name=ndb-e2e-test --up --down)

# Use the k8s 1.18.2 image built for KinD 0.8.0
# https://github.com/kubernetes-sigs/kind/releases/tag/v0.8.0
KUBETEST2+=(--image-name="kindest/node:v1.18.2@sha256:7b27a6d0f2517ff88ba444025beae41491b016bc6af573ba467b70c5e8e0d85f")

# Root directory for test artifacts and kubeconfig
E2E_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
KUBETEST2+=(--artifacts="${E2E_DIR}/artifacts" --kubeconfig="${E2E_DIR}/artifacts/kubeconfig")

# Arguments to run the tests
# TODO : enable passing arguments to ginkgo and the tests via bash arguments
GINKGO_CMD_AND_ARGS="go run github.com/onsi/ginkgo/ginkgo e2e"
# Arguments to be passed to the testcase
GINKGO_TEST_ARGS="--kubeconfig=${E2E_DIR}/artifacts/kubeconfig"
KUBETEST2+=(--test=exec -- ${GINKGO_CMD_AND_ARGS} -- ${GINKGO_TEST_ARGS})

# Bring up cluster, run the test and tear down
# TODO make it more dynamic
echo "Running kubetest2..."
"${KUBETEST2[@]}"
