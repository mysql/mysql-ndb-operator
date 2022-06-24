#!/bin/bash

# Copyright (c) 2022, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Wait for the pod's hostname/IP to be updated in the DNS.
#
# The management and data nodes use DNS to resolve hostnames
# and allocate/verify nodeIds. The pod's IP address can also
# change after a restart. So, wait until the DNS returns the
# "correct" IP address, which is available in the NDB_POD_IP
# env variable.
echo "Waiting for DNS to be updated..."
while
  resolvedIP=$(getent hosts "${HOSTNAME}.${HOSTNAME%-*}.${NDB_POD_NAMESPACE}" | awk '{print $1}')
  [[ "${resolvedIP}" != "${NDB_POD_IP}" ]]
do
  :
done
echo "Done."
