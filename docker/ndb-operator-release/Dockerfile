# Copyright (c) 2022, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
FROM golang:1.19.0-alpine3.16 as builder
RUN apk add --no-cache make

WORKDIR /ndb-operator
COPY ./ /ndb-operator/
ARG gitCommit
RUN ARCH=amd64 OS=linux WITH_DEBUG=0 GIT_COMMIT_ID=$gitCommit ./hack/build.sh

FROM oraclelinux:8-slim

COPY --from=builder /ndb-operator/bin/linux_amd64/* /usr/sbin/
COPY LICENSE .

ENTRYPOINT ["ndb-operator"]