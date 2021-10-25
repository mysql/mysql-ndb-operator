#!/bin/bash

# Copyright (c) 2021, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Health check script used by the startup probe of the MySQL Cluster data nodes

# Note : This script uses ndb_mgm to check if a data node has started.
#        This implies that a Management node has to be ready and available for
#        the script to succeed. Due to this reason, this script should be used
#        only as a startup probe. Using this as a liveness probe, for example,
#        will falsely report the data node as dead if the Management nodes are
#        not available.

# Usage : datanode-healthcheck.sh <connectstring> <data node start nodeId>

# Read and validate the arguments
connectstring="${1}"
dataNodeStartNodeId="${2}"
if [[ -z "${connectstring}" ]] || [[ -z "${dataNodeStartNodeId}" ]]; then
  echo "Please pass valid connectstring and data node start nodeId."
  echo "Usage : "
  echo "  ${0} <connectstring> <data node start nodeId>"
  exit 1
fi

# nodeId of data node running in current pod = dataNodeStartNodeId + statefulset pod ordinal index
sfsetPodOrdinalIdx=${HOSTNAME##*-}
nodeId=$((dataNodeStartNodeId+sfsetPodOrdinalIdx))

# Get node status using `ndb_mgm -e "<nodeId> status"` command
nodeStatus=$(ndb_mgm -c "${connectstring}" -e "${nodeId} status" --connect-retries=1)
# If nodeStatus has "Node ${nodeId}: started", the data node can be considered live and ready
if ! [[ "${nodeStatus}" =~ .*Node\ "${nodeId}":\ started.* ]]; then
  echo "Datanode health check failed."
  echo "Node status output : "
  echo "${nodeStatus}"
  exit 1
fi
