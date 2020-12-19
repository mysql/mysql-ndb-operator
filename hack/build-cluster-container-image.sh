#!/usr/bin/env bash

# Copyright (c) 2020, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Script to build a custom MySQL Cluster container image

# BASEDIR must be set
if [ -z "${BASEDIR}" ]; then
    echo "Please set MySQL Cluster build or install directory in BASEDIR"
    exit 1
fi

# Check that BASEDIR and BINDIR directories exist
BINDIR="${BASEDIR}/bin"
if [[ ! -d "${BASEDIR}" || ! -d "${BINDIR}" ]]; then
    echo "Please set a valid MySQL Cluster build or install directory in BASEDIR"
    exit 1
fi

# IMAGE_TAG must be set
if [ -z "${IMAGE_TAG}" ]; then
    echo "Please specify a tag for the container image in IMAGE_TAG"
    exit 1
fi

# Executable files to be copied inside the Docker image
exes_to_sbin=("ndbmtd" "ndb_mgmd" "mysqld" "mysqladmin")
exes_to_bin=("ndb_mgm" "mysql" "mysql_tzinfo_to_sql")

# copy all the required binaries to docker context
DOCKER_CTX_FILES="cluster-docker-files"
mkdir "${DOCKER_CTX_FILES}"

DOCKER_CTX_SBIN="${DOCKER_CTX_FILES}/sbin"
mkdir "${DOCKER_CTX_SBIN}"
for exe in "${exes_to_sbin[@]}"
do
  exe_path=${BINDIR}/${exe}
  cp "${exe_path}" "${DOCKER_CTX_SBIN}"
  chmod 755 "${DOCKER_CTX_SBIN}/${exe}"
done

DOCKER_CTX_BIN="${DOCKER_CTX_FILES}/bin"
mkdir "${DOCKER_CTX_BIN}"
for exe in "${exes_to_bin[@]}"
do
  exe_path=${BINDIR}/${exe}
  cp "${exe_path}" "${DOCKER_CTX_BIN}"
  chmod 755 "${DOCKER_CTX_BIN}/${exe}"
done

# Build container image
docker build -t mysql-cluster:"${IMAGE_TAG}" -f docker/Dockerfile .

# Cleanup all copied binaries
rm -rf "${DOCKER_CTX_FILES}"
