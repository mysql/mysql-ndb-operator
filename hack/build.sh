#!/bin/sh

# Copyright (c) 2020, 2022, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

set -o errexit
set -o nounset

# Required env variables :
# ARCH - target golang architecture
# OS   - target golang OS
# WITH_DEBUG - enable ndb operator debug build mode

PKG=$(go list -m)
VERSION=$(cat VERSION)

BUILD_TAGS=
if [ "${WITH_DEBUG}" = "1" ] || [ "${WITH_DEBUG}" = "ON" ]; then
  BUILD_TAGS="debug"
else
  BUILD_TAGS="release"
fi

echo "Building ndb operator..."
echo "arch:     ${ARCH}"
echo "os:       ${OS}"
echo "version:  ${VERSION}"
echo "pkg:      ${PKG}"
echo "build:    ${BUILD_TAGS}"

export CGO_ENABLED=0
export GOARCH="${ARCH}"
export GOOS="${OS}"

LDFLAGS="-X '${PKG}/config.version=${VERSION}'"
LDFLAGS="${LDFLAGS} -X '${PKG}/config.gitCommit=${GIT_COMMIT_ID}'"

BINARIES="./bin/${OS}_${ARCH}"
mkdir -p "${BINARIES}"

go build                    \
    -v                      \
    -o "${BINARIES}"        \
    -ldflags  "${LDFLAGS}"  \
    -tags "${BUILD_TAGS}"   \
    ./...
