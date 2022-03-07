#!/bin/sh

# Copyright (c) 2022, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Entrypoint script to start the ginkgo tests and watch for any SIGTERM signals.

# PID of the ginkgo process that will be started by this script
ginkgoPid=0

# Setup handler for SIGTERM signal
handleSigterm() {
  echo "Test is being aborted.."
  # touch /tmp/abort file to skip any upcoming suite runs
  touch /tmp/abort
  # we have to forward the signal to the test process
  # whose executable name will be '<suite-name>.test'
  if testPid=$(pgrep test$); then
    # pid exists - send SIGTERM to the test process and wait
    kill -s TERM "${testPid}"
    wait ${ginkgoPid}
  else
    # pid doesn't exist => the tests still are compiling
    # nothing to cleanup. exit.
    exit 1
  fi
}
trap handleSigterm TERM

# Run the ginkgo command passed to the script
"${@}" &
ginkgoPid=$!

# wait for the ginkgo tests to complete
wait ${ginkgoPid}
