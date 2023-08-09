# Copyright (c) 2021, 2023, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Compiles the given MySQL Source code inside
# a docker container running Oracle Linux 8.
# Note : This Dockerfile is designed to be used by the build.sh script
# in the parent directory and might not work as intended if used directly.
FROM container-registry.oracle.com/os/oraclelinux:8-slim

# Install the tools required for building MySQL
RUN echo "[main]" > /etc/dnf/dnf.conf \
    && microdnf update \
    && microdnf install cmake git make gcc gcc-c++ diffutils \
     openssl-devel openldap-devel cyrus-sasl-devel cyrus-sasl-scram libudev-devel \
     ncurses-devel libcurl-devel bison libtirpc-devel java-17-openjdk-devel \
    && microdnf --enablerepo=ol8_codeready_builder install rpcgen

VOLUME /mysql-cluster /build
WORKDIR /build

# Setup container to compile and build when run
CMD cmake -DWITH_NDBCLUSTER=1 -DCMAKE_BUILD_TYPE=Relwithdebinfo \
          -DCMAKE_C_COMPILER=gcc -DCMAKE_CXX_COMPILER=g++ \
          -DDOWNLOAD_BOOST=1 -DWITH_BOOST=/build/boost \
          -DWITH_NDBAPI_EXAMPLES=0 -Dprotobuf_BUILD_SHARED_LIBS=OFF \
          -DWITH_AUTHENTICATION_FIDO=OFF \
          /mysql-cluster \
    && make

