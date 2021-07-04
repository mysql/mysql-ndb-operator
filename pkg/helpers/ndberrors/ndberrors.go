// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndberrors

import (
	"errors"
)

const (
	// ErrReasonInvalidConfiguration is used if the Ndb resources as a configuration
	// that has flaws
	ErrReasonInvalidConfiguration = "InvalidConfiguration"

	// ErrReasonNoManagementServerConnection is used if the management server
	// can't be reached or connected to
	ErrReasonNoManagementServerConnection = "NoManagementServerConnection"

	// ErrReasonUnknown is placeholder for everything that is unknown
	ErrReasonUnknown = "ReasonUnknown"
)

// NdbError is the common Ndb operator error struct
type NdbError struct {

	// code is the unique error identifying number
	code int

	// reason is the description of why this bug occurs - e.g. illegal config
	reason string

	// details is the message given to the user / ui / log
	// e.g. number of nodes configured does not fit level of reduncancy
	details string

	// mitigation gives a possibly counter measure to fix the problem
	mitigation string
}

// Ensure that NdbError implements error interface
var _ error = &NdbError{}

// implement the Error() method of the error interface
func (err *NdbError) Error() string {
	return err.details
}

func newError() *NdbError {
	return &NdbError{}
}

func getReason(err error) string {
	if ndberr := newError(); errors.As(err, &ndberr) {
		return ndberr.reason
	}
	return ErrReasonUnknown
}

// IsInvalidConfiguration checks if an error is of type InvalidConfiguration
func IsInvalidConfiguration(err error) bool {
	return getReason(err) == ErrReasonInvalidConfiguration
}

// NewErrorNoManagementServerConnection creates a new NdbError of type invalid configuration
func NewErrorNoManagementServerConnection(message string) *NdbError {
	return &NdbError{
		reason:  ErrReasonNoManagementServerConnection,
		details: message,
	}
}

// IsNoManagementServerConnection tests if NdbError is of type no management server connection
func IsNoManagementServerConnection(err error) bool {
	return getReason(err) == ErrReasonNoManagementServerConnection
}
