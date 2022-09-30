#!/usr/bin/env bash

# Copyright (c) 2020, 2022, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

set -o errexit
set -o nounset
set -o pipefail

# install the required generators in go cache
go install k8s.io/code-generator/cmd/{defaulter-gen,client-gen,lister-gen,informer-gen,deepcopy-gen}

# get code-generator version from go.mod
SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
MODULE=$(grep "k8s.io/code-generator" ${SCRIPT_ROOT}/go.mod | xargs)
MODULE="${MODULE/ /@}"

# get absolute path to the module inside go mod cache
CODEGEN_PKG=$(go env GOMODCACHE)"/${MODULE}"

# Verify that the module exists
ls -d -1 "$CODEGEN_PKG"

OUTPUT_BASE="$(dirname "${BASH_SOURCE[0]}")/.."
PROJECT_MODULE=$(go list -m)

echo "output base is ${OUTPUT_BASE}"
echo "code gen pkg is ${CODEGEN_PKG}"
# generate the code with:
# --output-base    because this script should also be able to run inside the vendor dir of
#                  k8s.io/kubernetes. The output-base is needed for the generators to output into the vendor dir
#                  instead of the $GOPATH directly. For normal projects this can be dropped.
# "deepcopy,client,informer,lister" \
bash "${CODEGEN_PKG}"/generate-groups.sh \
  "deepcopy,client,informer,lister" \
  "${PROJECT_MODULE}/pkg/generated" \
  "${PROJECT_MODULE}/pkg/apis" \
  ndbcontroller:v1 \
  --output-base  "${OUTPUT_BASE}" \
  --go-header-file "${SCRIPT_ROOT}"/hack/ndb-boilerplate.go.txt

#  "${PROJECT_MODULE}/pkg/apis" \

# IMPORTANT
# all code needs to be generated with the following PROJECT_MODULE module name
# and will thus be generated in the directory named the same
# move stuff from there
cp -r ${PROJECT_MODULE}/pkg/generated pkg/
cp ${PROJECT_MODULE}/pkg/apis/ndbcontroller/v1/zz_generated.deepcopy.go \
    pkg/apis/ndbcontroller/v1/zz_generated.deepcopy.go
rm -rf $(echo ${PROJECT_MODULE} | awk -F "/" '{print $1}')
