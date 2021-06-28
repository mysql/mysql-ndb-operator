#!/bin/sh

# Copyright (c) 2020, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

set -o errexit
set -o nounset

# Required env variables :
# ARCH - target golang architecture
# OS   - target golang OS

PKG=$(go list -m)
VERSION=$(cat VERSION)

echo "Building ndb operator..."
echo "arch:     ${ARCH}"
echo "os:       ${OS}"
echo "version:  ${VERSION}"
echo "pkg:      ${PKG}"

export CGO_ENABLED=0
export GOARCH="${ARCH}"
export GOOS="${OS}"

LDFLAGS="-X '${PKG}/config.version=${VERSION}'"
LDFLAGS="${LDFLAGS} -X '${PKG}/config.gitCommit=$(git rev-parse --short HEAD)'"

BINARIES="./bin/${OS}_${ARCH}"
mkdir -p "${BINARIES}"

go build                    \
    -v                      \
    -o "${BINARIES}"        \
    -ldflags  "${LDFLAGS}"  \
    ./...
