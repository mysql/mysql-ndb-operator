// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package webhook

import (
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"

	v1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/klog"
)

func notAllowedAdmissionResponse(reqUID types.UID, statusError *errors.StatusError) *v1.AdmissionResponse {
	return &v1.AdmissionResponse{
		UID:     reqUID,
		Allowed: false,
		Result:  &statusError.ErrStatus,
	}
}

// requestDenied returns a AdmissionResponse with the request denied
func requestDenied(reqUID types.UID, statusError *errors.StatusError) *v1.AdmissionResponse {
	errStatus := &statusError.ErrStatus
	klog.Infof("Request denied (%s) : %s", errStatus.Reason, errStatus.Message)
	return notAllowedAdmissionResponse(reqUID, statusError)
}

// requestDeniedBad returns a AdmissionResponse with the request denied and with reason StatusReasonBadRequest
func requestDeniedBad(reqUID types.UID, errMsg string) *v1.AdmissionResponse {
	return requestDenied(reqUID, errors.NewBadRequest(errMsg))
}

// requestDeniedNdbInvalid returns a AdmissionResponse with the request denied and with reason StatusReasonInvalid
func requestDeniedNdbInvalid(
	reqUID types.UID, ndb *v1alpha1.Ndb, errs field.ErrorList) *v1.AdmissionResponse {
	return requestDenied(reqUID, errors.NewInvalid(ndb.GroupVersionKind().GroupKind(), ndb.Name, errs))
}

// requestAllowed returns a AdmissionResponse with the request allowed
func requestAllowed(reqUID types.UID) *v1.AdmissionResponse {
	klog.Info("Request allowed")
	return &v1.AdmissionResponse{
		UID:     reqUID,
		Allowed: true,
	}
}
