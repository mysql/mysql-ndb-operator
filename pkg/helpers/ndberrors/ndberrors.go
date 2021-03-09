package ndberrors

import (
	"errors"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// map of invalid fields and the related error messages
	// this will be set only if the ndbError is of type ErrReasonInvalidConfiguration
	fieldDetails []metav1.StatusCause
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

func GetFieldDetails(err error) []metav1.StatusCause {
	if ndberr := newError(); errors.As(err, &ndberr) {
		return ndberr.fieldDetails
	}
	return nil
}

// invalidConfigNdbErrorBuilder is used to build a NdbError with reason ErrReasonInvalidConfiguration
type invalidConfigNdbErrorBuilder struct {
	fieldDetails []metav1.StatusCause
}

// AddInvalidField adds an invalid field name and the error message to the fieldDetails
func (b *invalidConfigNdbErrorBuilder) AddInvalidField(invalidFieldName, invalidValue, message string) {
	b.fieldDetails = append(b.fieldDetails, metav1.StatusCause{
		Type: metav1.CauseTypeFieldValueInvalid,
		// Use a format message similar to the standard K8s validation messages
		Message: fmt.Sprintf("Invalid value: %s: %s", invalidValue, message),
		Field:   invalidFieldName,
	})
}

// NdbError returns a NdbError filled with information from the builder
func (b *invalidConfigNdbErrorBuilder) NdbError() error {
	// generate error message from the invalidFields
	var errMsg string
	switch len(b.fieldDetails) {
	case 0:
		// There is no error
		return nil
	case 1:
		for _, fd := range b.fieldDetails {
			errMsg = fmt.Sprintf("Invalid value for field %s: %s", fd.Field, fd.Message)
		}
		break
	default:
		// multiple invalid field values
		errMsg = "The Ndb resource has invalid field values :\n"
		for _, fd := range b.fieldDetails {
			errMsg = fmt.Sprintf("* %s: %s\n", fd.Field, fd.Message)
		}
	}

	// return a NdbError with filled in details
	return &NdbError{
		reason:       ErrReasonInvalidConfiguration,
		details:      errMsg,
		fieldDetails: b.fieldDetails,
	}

}

// NewInvalidConfigNdbErrorBuilder returns a new invalid config error builder
func NewInvalidConfigNdbErrorBuilder() *invalidConfigNdbErrorBuilder {
	return &invalidConfigNdbErrorBuilder{}
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
