# Copyright (c) 2021, 2023, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Builds custom MySQL Cluster minimal docker image to be consumed
# by the Ndb operator from precompiled binaries for ol8.
# Note : This Dockerfile is designed to be used by the build.sh script
# in the same directory and might not work as intended if used directly.
FROM container-registry.oracle.com/os/oraclelinux:8-slim

# Setup microdnf and required tools
RUN echo "[main]" > /etc/dnf/dnf.conf \
    && microdnf update
    # && microdnf install openssl-libs procps

# Create the directories used by mysqld
COPY prepare-image.sh /
RUN /prepare-image.sh && rm -f /prepare-image.sh

COPY sbin/* /usr/sbin/
COPY bin/* /usr/bin/

EXPOSE 3306 1186 8080

# Run the container as MySQL User
RUN /usr/sbin/groupadd -g 27 -o -r mysql \
    && /usr/sbin/useradd -M -N -g mysql -o -r -s /bin/false -c "MySQL Server" -u 27 mysql

CMD ["ndb_mgmd", "--foreground"]

