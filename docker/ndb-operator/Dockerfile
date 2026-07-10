# Copyright (c) 2020, 2026, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

FROM container-registry.oracle.com/os/oraclelinux:9-slim

# Copy only binaries intended for the production operator image.
COPY bin/linux_amd64/ndb-operator \
     bin/linux_amd64/ndb-operator-webhook \
     bin/linux_amd64/ndb-pod-initializer \
     /usr/sbin/

ENTRYPOINT ["ndb-operator"]
