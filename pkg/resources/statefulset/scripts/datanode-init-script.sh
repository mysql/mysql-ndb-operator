#!/bin/bash

# Copyright (c) 2022, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Data node pod initialisation script

# Extract the nodeId written by the init container
nodeId=$(cat /var/lib/ndb/run/nodeId.val)

# Check if all live data nodes have completed handling
# this node's previous failure.
#
# Use ndb_waiter to check if there are any live nodes,
# i.e. nodes with status 'STARTED'.
# Note : ndb_waiter prints the Node status multiple times
# within a second and startedDataNodeCount might be more
# than the total number of Data Nodes. This is okay as all
# we are interested in is if there are any live data nodes
# in the system and not the exact count of them.
startedDataNodeCount=$(ndb_waiter -c "${NDB_CONNECTSTRING}" --timeout=1 2>/dev/null \
                          | grep -c -E "Node [0-9]+: STARTED")
if ((startedDataNodeCount > 0)); then
  # Some data nodes are already 'STARTED' and available.
  # Check if they are still handling the previous failure
  # (or stop) of this node. This is done by querying the
  # status of this node from the ndbinfo.restart_info table.
  # It will be set to "Node failure handling complete" when
  # all live nodes complete handling the previous Node failure.
  # If not complete, wait until it is completed.
  echo "Waiting for other nodes to complete Node failure handling..."
  while
    nodeFailureHandlingComplete=$(ndbinfo_select_all -c "${NDB_CONNECTSTRING}" --connect-retries=1 restart_info \
                      | awk -v id="${nodeId}" '$1 == id' \
                      | grep -c "Node failure handling complete")
    ((nodeFailureHandlingComplete == 0))
  do
    :
  done
  echo "Done."
fi

echo "Data Node init script succeeded."
