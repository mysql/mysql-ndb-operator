// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package webhook

import (
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	klog "k8s.io/klog/v2"
)

func notAllowedAdmissionResponse(reqUID types.UID, statusError *errors.StatusError) *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{
		UID:     reqUID,
		Allowed: false,
		Result:  &statusError.ErrStatus,
	}
}

// requestDenied returns a AdmissionResponse with the request denied
func requestDenied(reqUID types.UID, statusError *errors.StatusError) *admissionv1.AdmissionResponse {
	errStatus := &statusError.ErrStatus
	klog.Infof("Request denied (%s) : %s", errStatus.Reason, errStatus.Message)
	return notAllowedAdmissionResponse(reqUID, statusError)
}

// requestDeniedBad returns a AdmissionResponse with the request denied and with reason StatusReasonBadRequest
func requestDeniedBad(reqUID types.UID, errMsg string) *admissionv1.AdmissionResponse {
	return requestDenied(reqUID, errors.NewBadRequest(errMsg))
}

// requestDeniedNdbInvalid returns a AdmissionResponse with the request denied and with reason StatusReasonInvalid
func requestDeniedNdbInvalid(
	reqUID types.UID, nc *v1.NdbCluster, errs field.ErrorList) *admissionv1.AdmissionResponse {
	return requestDenied(reqUID, errors.NewInvalid(nc.GroupVersionKind().GroupKind(), nc.Name, errs))
}

// requestAllowed returns a AdmissionResponse with the request allowed
func requestAllowed(reqUID types.UID) *admissionv1.AdmissionResponse {
	klog.Info("Request allowed")
	return &admissionv1.AdmissionResponse{
		UID:     reqUID,
		Allowed: true,
	}
}

// requestAllowedWithPatch returns a AdmissionResponse with the
// request allowed response and a patch to be applied to the object
func requestAllowedWithPatch(reqUID types.UID, patch []byte) *admissionv1.AdmissionResponse {
	klog.Info("Request allowed")
	patchType := admissionv1.PatchTypeJSONPatch
	return &admissionv1.AdmissionResponse{
		UID:       reqUID,
		Allowed:   true,
		Patch:     patch,
		PatchType: &patchType,
	}
}
