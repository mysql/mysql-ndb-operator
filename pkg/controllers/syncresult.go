// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import "time"

// sync result describes how to continue after a synchronization step in the sync handler
type syncResult interface {
	// finished true means that step is completed for this round and sync handler shall exit
	// if finished is false then proceed to next sync step
	finished() bool

	// requeue means that sync handler shall exit but report after duration seconds
	requeueIn() time.Duration

	// return the error if there is one
	getError() error
}

type finishedResult struct{}
type continueResult struct{}
type requeueResult struct {
	secsToRequeue int
}
type errorResult struct {
	err error
}

func (r finishedResult) finished() bool           { return true }
func (r finishedResult) requeueIn() time.Duration { return 0 }
func (r finishedResult) getError() error          { return nil }

func (r continueResult) finished() bool           { return false }
func (r continueResult) requeueIn() time.Duration { return 0 }
func (r continueResult) getError() error          { return nil }

func (r requeueResult) finished() bool { return true }
func (r requeueResult) requeueIn() time.Duration {
	return time.Duration(r.secsToRequeue) * time.Second
}
func (r requeueResult) getError() error { return nil }

func (r errorResult) finished() bool           { return true }
func (r errorResult) requeueIn() time.Duration { return 0 }
func (r errorResult) getError() error          { return r.err }

func finishProcessing() syncResult {
	return finishedResult{}
}

func continueProcessing() syncResult {
	return continueResult{}
}

func requeueInSeconds(secs int) syncResult {
	return requeueResult{secsToRequeue: secs}
}

func errorWhileProcssing(err error) syncResult {
	return errorResult{err: err}
}
