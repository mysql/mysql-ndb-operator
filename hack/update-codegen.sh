#!/usr/bin/env bash

# Copyright (c) 2020, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/..

# get go.mod
GOMOD=$(grep "k8s.io/code-generator => " ${SCRIPT_ROOT}/go.mod)

# extract directory and version in gopath
MODULE=$(awk '{split($0, a, " => "); print a[2]}' <<< ${GOMOD})
MODULE="${MODULE/ /@}"

# install the required generators in go cache
go install k8s.io/code-generator/cmd/{defaulter-gen,client-gen,lister-gen,informer-gen,deepcopy-gen}

# get go module cache
CACHE=$(go env GOMODCACHE)

CODEGEN_PKG=${CODEGEN_PKG:-$(cd "${SCRIPT_ROOT}"; ls -d -1 ${CACHE}/${MODULE} 2>/dev/null || echo ../code-generator)}
#OUTPUT_BASE="$(dirname "${BASH_SOURCE[0]}")/../../.."
OUTPUT_BASE="$(dirname "${BASH_SOURCE[0]}")/.."

# mind the missing slash 
# - listers and informers are not generated 
#   without 100% clean paths (e.g. // in path)
PROJECT_MODULE="github.com/mysql/ndb-operator"

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
  ndbcontroller:v1alpha1 \
  --output-base  "${OUTPUT_BASE}" \
  --go-header-file "${SCRIPT_ROOT}"/hack/ndb-boilerplate.go.txt

#  "${PROJECT_MODULE}/pkg/apis" \

# IMPORTANT
# all code needs to be generated with the following PROJECT_MODULE module name
# and will thus be generated in the directory named the same
# move stuff from there
cp -r ${PROJECT_MODULE}/pkg/generated pkg/
cp ${PROJECT_MODULE}/pkg/apis/ndbcontroller/v1alpha1/zz_generated.deepcopy.go \
    pkg/apis/ndbcontroller/v1alpha1/zz_generated.deepcopy.go
rm -rf $(echo ${PROJECT_MODULE} | awk -F "/" '{print $1}')
