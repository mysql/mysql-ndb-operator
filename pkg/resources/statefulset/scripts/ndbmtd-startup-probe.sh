#!/bin/bash

# Copyright (c) 2021, 2022, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Health check script used by the startup probe of the MySQL Cluster data nodes

# Note : This script uses ndb_mgm to check if a data node has started.
#        This implies that a Management node has to be ready and available for
#        the script to succeed. Due to this reason, this script should be used
#        only as a startup probe. Using this as a liveness probe, for example,
#        will falsely report the data node as dead if the Management nodes are
#        not available.

# Extract the nodeId written by the init container
nodeId=$(cat /var/lib/ndb/run/nodeId.val)

# Get node status using `ndb_mgm -e "<nodeId> status"` command
nodeStatus=$(ndb_mgm -c "${NDB_CONNECTSTRING}" -e "${nodeId} status" --connect-retries=1)
# If nodeStatus has "Node ${nodeId}: started", the data node can be considered live and ready
if ! [[ "${nodeStatus}" =~ .*Node\ "${nodeId}":\ started.* ]]; then
  echo "Datanode health check failed."
  echo "Node status output : "
  echo "${nodeStatus}"
  exit 1
fi
