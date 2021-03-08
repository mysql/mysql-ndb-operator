package webhook

import (
	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"
)

func failureStatus(
	errMsg string, reason metav1.StatusReason, details *metav1.StatusDetails) *metav1.Status {
	return &metav1.Status{
		Status:  metav1.StatusFailure,
		Message: errMsg,
		Reason:  reason,
		Details: details,
	}
}

func notAllowedAdmissionResponse(reqUID types.UID, result *metav1.Status) *v1.AdmissionResponse {
	return &v1.AdmissionResponse{
		UID:     reqUID,
		Allowed: false,
		Result:  result,
	}
}

// requestDenied returns a AdmissionResponse with the request denied
func requestDenied(reqUID types.UID, result *metav1.Status) *v1.AdmissionResponse {
	klog.Infof("Request denied (%s) : %s", result.Reason, result.Message)
	return notAllowedAdmissionResponse(reqUID, result)
}

// requestDeniedBad returns a AdmissionResponse with the request denied due to BadRequest
func requestDeniedBad(reqUID types.UID, errMsg string) *v1.AdmissionResponse {
	return requestDenied(reqUID, failureStatus(errMsg, metav1.StatusReasonBadRequest, nil))
}

// requestAllowed returns a AdmissionResponse with the request allowed
func requestAllowed(reqUID types.UID) *v1.AdmissionResponse {
	klog.Info("Request allowed")
	return &v1.AdmissionResponse{
		UID:     reqUID,
		Allowed: true,
	}
}
