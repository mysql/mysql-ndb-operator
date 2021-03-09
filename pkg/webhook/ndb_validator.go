package webhook

import (
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/helpers"
	"github.com/mysql/ndb-operator/pkg/helpers/ndberrors"

	v1 "k8s.io/api/admission/v1"
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

func getStatusDetails(ndb *v1alpha1.Ndb) *metav1.StatusDetails {
	return &metav1.StatusDetails{
		Name:  ndb.Name,
		Group: "mysql.oracle.com",
		Kind:  "Ndb",
		UID:   ndb.GetUID(),
	}
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
	if err := helpers.IsValidConfig(ndb); err != nil {
		// ndb does not have a valid configuration
		// get more details about the error and send it back
		details := getStatusDetails(ndb)
		details.Causes = ndberrors.GetFieldDetails(err)
		return requestDenied(reqUID, failureStatus(err.Error(), metav1.StatusReasonInvalid, details))
	}

	return requestAllowed(reqUID)
}

func (nv *ndbValidator) validateUpdate(
	reqUID types.UID, obj runtime.Object, oldObj runtime.Object) *v1.AdmissionResponse {
	// TODO: properly validate obj w.r.to existing oldObj and
	//       check if any previous change is still being applied
	return nv.validateCreate(reqUID, obj)
}
