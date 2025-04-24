#!/usr/bin/env bash

# Copyright (c) 2020, 2025, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

set -o errexit
set -o nounset
set -o pipefail

go get k8s.io/code-generator

# get code-generator version from go.mod
SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
MODULE=$(grep "k8s.io/code-generator" "${SCRIPT_ROOT}/go.mod" | xargs)
MODULE="${MODULE/ /@}"

# get absolute path to the module inside go mod cache
CODEGEN_PKG=$(go env GOMODCACHE)"/${MODULE}"

# Verify that the module exists
if [ ! -d "${CODEGEN_PKG}" ]; then
  echo "k8s.io/code-generator module not found in '${CODEGEN_PKG}'"
  exit 1
fi

OUTPUT_BASE="$(dirname "${BASH_SOURCE[0]}")/.."
PROJECT_MODULE=$(go list -m)

echo "output base is ${OUTPUT_BASE}"
echo "code gen pkg is ${CODEGEN_PKG}"

source "${CODEGEN_PKG}/kube_codegen.sh"

kube::codegen::gen_helpers \
  --boilerplate "${SCRIPT_ROOT}/hack/ndb-boilerplate.go.txt" \
  "${SCRIPT_ROOT}/pkg/apis"

kube::codegen::gen_client \
  --boilerplate "${SCRIPT_ROOT}/hack/ndb-boilerplate.go.txt" \
  --with-watch \
  --output-dir "${OUTPUT_BASE}/pkg/generated" \
  --output-pkg "${PROJECT_MODULE}/pkg/generated" \
  "${SCRIPT_ROOT}/pkg/apis"
