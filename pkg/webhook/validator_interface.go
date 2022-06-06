// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package webhook

import (
	"fmt"
	"k8s.io/apimachinery/pkg/types"
	"regexp"

	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
)

type validator interface {
	getGVR() *metav1.GroupVersionResource
	getGVK() *schema.GroupVersionKind
	newObject() runtime.Object
	// validate functions should validate the request and return a AdmissionResponse
	validateCreate(reqUID types.UID, obj runtime.Object) *v1.AdmissionResponse
	validateUpdate(reqUID types.UID, obj runtime.Object, oldObj runtime.Object) *v1.AdmissionResponse
}

func unsupportedOperation(reqUID types.UID, operation v1.Operation) *v1.AdmissionResponse {
	errMsg := fmt.Sprintf("validating a %s operation not supported", operation)
	klog.Error(errMsg)
	return requestDeniedBad(reqUID, errMsg)
}

func validate(req *v1.AdmissionRequest, v validator) *v1.AdmissionResponse {
	// Verify right resource is passed
	resource := v.getGVR()
	if req.Resource != *resource {
		errMsg := fmt.Sprintf("expected resource %v but got %v", *resource, req.Resource)
		return requestDeniedBad(req.UID, errMsg)
	}

	// Handle operation
	defaultGVK := v.getGVK()
	decoder := scheme.Codecs.UniversalDeserializer()
	switch req.Operation {
	case v1.Create:
		// retrieve new object and validate it
		obj, _, err := decoder.Decode(req.Object.Raw, defaultGVK, v.newObject())
		if err != nil {
			return requestDeniedBad(req.UID, err.Error())
		}
		klog.V(5).Info(fmt.Sprintf("Retrieved new object : %v", obj))
		return v.validateCreate(req.UID, obj)

	case v1.Update:
		// any updates made from the ndb-operator can be accepted without validation
		if updateFromNdbOperator, _ := regexp.MatchString(
			"system:serviceaccount:.*:ndb-operator", req.UserInfo.Username); updateFromNdbOperator {
			klog.Info("Skipping validation for an update from ndb-operator")
			return requestAllowed(req.UID)
		}

		// retrieve new and old objects
		obj, _, err := decoder.Decode(req.Object.Raw, defaultGVK, v.newObject())
		if err != nil {
			return requestDeniedBad(req.UID, err.Error())
		}
		klog.V(5).Info(fmt.Sprintf("Retrieved new object : %v", obj))

		oldObject, _, err := decoder.Decode(req.OldObject.Raw, defaultGVK, v.newObject())
		if err != nil {
			return requestDeniedBad(req.UID, err.Error())
		}
		klog.V(5).Info(fmt.Sprintf("Retrieved old object : %v", oldObject))

		// validate the update
		return v.validateUpdate(req.UID, obj, oldObject)

	default:
		return unsupportedOperation(req.UID, req.Operation)
	}
}
