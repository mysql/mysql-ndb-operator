// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package signals

import (
	"context"
	"os"
	"os/signal"
)

var onlyOneSignalHandler = make(chan struct{})

// SetupSignalHandler registers for SIGTERM and SIGINT signals and
// returns a context that will be cancelled on receiving either of
// these signals. If a second signal is caught, the program is
// terminated with exit code 1.
func SetupSignalHandler(parentCtx context.Context) (ctx context.Context) {
	close(onlyOneSignalHandler) // panics when called twice

	ctx, cancel := context.WithCancel(parentCtx)
	c := make(chan os.Signal, 2)
	signal.Notify(c, shutdownSignals...)
	go func() {
		// First signal cancels the context
		<-c
		cancel()
		// Second signal causes the program to exit
		<-c
		os.Exit(1)
	}()

	return ctx
}
