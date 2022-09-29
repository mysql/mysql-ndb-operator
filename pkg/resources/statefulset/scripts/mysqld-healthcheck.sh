#!/bin/bash

# Copyright (c) 2021, 2022, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Health check script - used by the health probes of the MySQL Server image

set -o errexit

# Location of the required script and cnf is passed as the first arg to the script
healthCheckCnf="${1}/healthcheck.cnf"
mysqlInitComplete="${1}/mysql-init-complete"

# Entrypoint script in docker image touches the mysql-init-complete file after
# the initialisation is complete and before the main server process is started.
if [ ! -f "${mysqlInitComplete}" ]; then
  # initialisation is not complete yet
  exit 1
fi

# MySQL initialisation is complete
# Check if the MySQL Cluster binlog setup is ongoing
setupDone=$(mysql --defaults-extra-file="${healthCheckCnf}" \
                  -NB -e 'select mysql.IsNdbclusterSetupComplete()')
if [ "${setupDone}" -ne 1 ]; then
  # Binlog setup is ongoing => ndb engine is not ready for query
  exit 1
fi
