# Copyright (c) 2022, 2026, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/
ARG GOLANG_ALPINE_IMAGE=docker.io/golang:1.25.4-alpine3.22
FROM $GOLANG_ALPINE_IMAGE as builder
RUN apk add --no-cache make

WORKDIR /ndb-operator
COPY ./ /ndb-operator/
ARG gitCommit
RUN ARCH=amd64 OS=linux WITH_DEBUG=0 GIT_COMMIT_ID=$gitCommit ./hack/build.sh

FROM container-registry.oracle.com/os/oraclelinux:9-slim

# Copy only binaries intended for the production operator image.
COPY --from=builder /ndb-operator/bin/linux_amd64/ndb-operator \
                    /ndb-operator/bin/linux_amd64/ndb-operator-webhook \
                    /ndb-operator/bin/linux_amd64/ndb-pod-initializer \
                    /usr/sbin/
COPY --from=builder /ndb-operator/vendor/github.com/go-sql-driver/* /go-sql-driver-source
COPY LICENSE .

ENTRYPOINT ["ndb-operator"]
