# Copyright (c) 2020, 2021, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

FROM oraclelinux:8-slim

COPY bin/linux_amd64/* /usr/sbin/
COPY pkg/helpers/scripts/* /ndb-operator-scripts/

ENTRYPOINT ["ndb-operator"]
