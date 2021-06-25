package webhook

import (
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/helpers"
	v1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// ndbValidator implements validators for Ndb resource
type ndbValidator struct{}

func newNdbValidator() validator {
	return &ndbValidator{}
}

func (nv *ndbValidator) getGVR() *metav1.GroupVersionResource {
	return &metav1.GroupVersionResource{
		Group:    "mysql.oracle.com",
		Version:  "v1alpha1",
		Resource: "ndbs",
	}
}

func (nv *ndbValidator) getGVK() *schema.GroupVersionKind {
	return &schema.GroupVersionKind{
		Group:   "mysql.oracle.com",
		Version: "v1alpha1",
		Kind:    "ndb",
	}
}

func (nv *ndbValidator) newObject() runtime.Object {
	return &v1alpha1.Ndb{}
}

func (nv *ndbValidator) validateCreate(reqUID types.UID, obj runtime.Object) *v1.AdmissionResponse {
	ndb := obj.(*v1alpha1.Ndb)
	if errList := helpers.IsValidConfig(ndb, nil); errList != nil {
		// ndb does not define a valid configuration
		return requestDeniedNdbInvalid(reqUID, ndb, errList)
	}

	return requestAllowed(reqUID)
}

func (nv *ndbValidator) validateUpdate(
	reqUID types.UID, newObj runtime.Object, oldObj runtime.Object) *v1.AdmissionResponse {

	oldNdb := oldObj.(*v1alpha1.Ndb)
	if oldNdb.Status.ProcessedGeneration != oldNdb.Generation {
		// The previous update is still being applied, and
		// the operator can handle only one update at a moment.
		return requestDenied(reqUID,
			errors.NewTooManyRequestsError("previous update to the Ndb resource is still being applied"))
	}

	newNdb := newObj.(*v1alpha1.Ndb)
	if errList := helpers.IsValidConfig(newNdb, oldNdb); errList != nil {
		// new ndb does not define a valid configuration
		return requestDeniedNdbInvalid(reqUID, newNdb, errList)
	}

	return requestAllowed(reqUID)
}
