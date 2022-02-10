#!/bin/sh

# Copyright (c) 2022, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Entrypoint script to start the ginkgo tests and watch for any SIGTERM signals.

# Setup handler for SIGTERM signal
handleSigterm() {
  echo "Test is being aborted.."
  # touch /tmp/abort file to skip any upcoming suite runs
  touch /tmp/abort
  # we have to forward the signal to the test process
  # whose executable name will be '<suite-name>.test'
  if pid=$(pgrep test$); then
    # pid exists - send SIGTERM to the test process and wait
    kill -s TERM "${pid}"
    wait
  else
    # pid doesn't exist => the tests still are compiling
    # nothing to cleanup. exit.
    exit 1
  fi
}
trap handleSigterm TERM

# Run the actual command passed to the script
"${@}" &

# wait for the command to complete
wait
