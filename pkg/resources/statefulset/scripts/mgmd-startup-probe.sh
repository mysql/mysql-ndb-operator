#!/bin/bash

# Copyright (c) 2022, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Startup probe of the MySQL Cluster management nodes

set -e

# Extract the nodeId written by the init container
nodeId=$(cat /var/lib/ndb/run/nodeId.val)

# Get local mgmd status using `ndb_mgm -e "<nodeId> status"` command
nodeStatus=$(ndb_mgm -c "localhost" -e "${nodeId} status" --connect-retries=1)
# If nodeStatus has "Node ${nodeId}: connected", the management node can be considered ready
if ! [[ "${nodeStatus}" =~ .*Node\ "${nodeId}":\ connected.* ]]; then
  echo "Management node health check failed."
  echo "Node status output : "
  echo "${nodeStatus}"
  exit 1
fi

# This Management node is "ready" to accept connections.
# If this node is being restarted as a part of a the mgmd StatefulSet
# update, it should be considered fully "ready" only when all the other
# existing mgmd and data nodes have connected to it.
connectstringExcludingNodeId=${NDB_CONNECTSTRING#*,}
connectstrings=(${connectstringExcludingNodeId//,/ })
if ((${#connectstrings[@]} == 1)); then
  # No need to handle update case as there is only one mgmd
  # (i.e replica=1) and update is denied for such cases.
  exit 0
fi

# Deduce the nodeId and connectstring of the "other" mgmd
otherMgmdNodeId=0
otherMgmdConnectstring=""
if ((nodeId == 1)); then
  otherMgmdNodeId=2
  otherMgmdConnectstring=${connectstrings[1]}
else
  otherMgmdNodeId=1
  otherMgmdConnectstring=${connectstrings[0]}
fi

# Check if the other mgmd is already running.
if [[ $(getent hosts "${otherMgmdConnectstring%:*}" | awk '{print $1}') == "" ]]; then
  # DNS lookup returned empty string => the other mgmd pod doesn't exist yet
  # This is an ISR and this local mgmd is ready
  exit 0
fi

# Other mgmd is running. Local mgmd is ready when it reports the exact
# same status about the connected mgmd and data nodes as the other mgmd.
# Note : SQL/API node status is not compared and that seems to be okay for now.
clusterStatusFromLocalMgmd=$(ndb_mgm -c localhost:1186 --connect-retries=1 -e show)
clusterStatusFromOtherMgmd=$(ndb_mgm -c "${otherMgmdConnectstring}" --connect-retries=1 -e show)
# Compare Management nodes' status first
mgmdStatusFromLocalMgmd=$(echo "${clusterStatusFromLocalMgmd}" | sed -n '/ndb_mgmd(MGM)/,+2p')
mgmdStatusFromOtherMgmd=$(echo "${clusterStatusFromOtherMgmd}" | sed -n '/ndb_mgmd(MGM)/,+2p')
if [[ "${mgmdStatusFromLocalMgmd}" != "${mgmdStatusFromOtherMgmd}" ]]; then
  # Local Mgmd not ready
  exit 1
fi

# Compare Data node status
# Extract the number of data nodes from other mgmd
numOfNodes=$(echo "${clusterStatusFromOtherMgmd}" | grep -Po '\[ndbd\(NDB\)\]\t\K[0-9]+(?= node\(s\))')
# This management node might have new data nodes in its configuration
# but the other might not have it during online add node. So extract
# and compare only the status of first $numOfNodes data nodes from the status.
ndmtdStatusFromLocalMgmd=$(echo "${clusterStatusFromLocalMgmd}" | sed -n "/ndbd(NDB)/,+${numOfNodes}p" | sed '1d')
ndmtdStatusFromOtherMgmd=$(echo "${clusterStatusFromOtherMgmd}" | sed -n "/ndbd(NDB)/,+${numOfNodes}p" | sed '1d')
if [[ "${ndmtdStatusFromLocalMgmd}" != "${ndmtdStatusFromOtherMgmd}" ]]; then
  # Local Mgmd not ready
  exit 1
fi
