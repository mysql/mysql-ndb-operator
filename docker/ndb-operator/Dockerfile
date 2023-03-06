# Copyright (c) 2020, 2023, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

FROM container-registry.oracle.com/os/oraclelinux:8-slim

COPY bin/linux_amd64/* /usr/sbin/

ENTRYPOINT ["ndb-operator"]
