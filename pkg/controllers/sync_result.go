// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

// syncResult defines the common methods that the
// result of a synchronization step has to implement.
// On receiving a syncResult implementing type's object
// from a sync step, the sync and syncHandler methods
// will decide how to proceed further using the methods
// defined here.
type syncResult interface {
	// stopSync returns true if further synchronisation
	// needs to be stopped. This usually happens when a
	// sync step changes has been applied and the handler
	// has to wait sometime for that step to complete
	// before continuing.
	stopSync() bool

	// getError returns any error that occurred
	// during the sync step
	getError() error
}

// syncResultContinueProcessing implements the syncResult
// interface and should be returned by the sync steps after
// which synchronisation can continue further.
type syncResultContinueProcessing struct{}

func (r *syncResultContinueProcessing) stopSync() bool  { return false }
func (r *syncResultContinueProcessing) getError() error { return nil }

// syncResultStopProcessing implements the syncResult
// interface and should be returned by the sync steps
// after which synchronisation should not continue
// further.
type syncResultStopProcessing struct {
	syncResultContinueProcessing
}

func (r *syncResultStopProcessing) stopSync() bool { return true }

// syncResultErrorOccurred implements the syncResult
// interface and should be returned by the sync steps
// during which an error has occurred. Returning this
// will also cause the caller to stop further
// synchronisation.
type syncResultErrorOccurred struct {
	syncResultStopProcessing
	err error
}

func (r *syncResultErrorOccurred) getError() error { return r.err }

// helper methods to return SyncResult from sync step methods
func continueProcessing() syncResult {
	return &syncResultContinueProcessing{}
}

func finishProcessing() syncResult {
	return &syncResultStopProcessing{}
}

func errorWhileProcessing(err error) syncResult {
	return &syncResultErrorOccurred{err: err}
}
