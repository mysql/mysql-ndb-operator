#!/bin/bash

# Copyright (c) 2021, 2022, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Health check script - used by the health probes of the MySQL Server image

set -o errexit

# Location of the required cnf is passed as the first arg to the script
healthCheckCnf="${1}/healthcheck.cnf"

# Ping server to see if it is ready
mysqladmin --defaults-extra-file="${healthCheckCnf}" ping

# MySQL Server is ready
# Check if the MySQL Cluster binlog setup is ongoing
setupDone=$(mysql --defaults-extra-file="${healthCheckCnf}" \
                  -NB -e 'select mysql.IsNdbclusterSetupComplete()')
if [ "${setupDone}" -ne 1 ]; then
  # Binlog setup is ongoing => ndb engine is not ready for query
  exit 1
fi
