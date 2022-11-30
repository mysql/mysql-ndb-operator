// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package webhook

import (
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// ndbAdmissionController implements admissionController for Ndb resource
type ndbAdmissionController struct{}

func newNdbAdmissionController() admissionController {
	return &ndbAdmissionController{}
}

func (nv *ndbAdmissionController) getGVR() *metav1.GroupVersionResource {
	return &metav1.GroupVersionResource{
		Group:    "mysql.oracle.com",
		Version:  "v1",
		Resource: "ndbclusters",
	}
}

func (nv *ndbAdmissionController) getGVK() *schema.GroupVersionKind {
	return &schema.GroupVersionKind{
		Group:   "mysql.oracle.com",
		Version: "v1",
		Kind:    "ndbcluster",
	}
}

func (nv *ndbAdmissionController) newObject() runtime.Object {
	return &v1.NdbCluster{}
}

func (nv *ndbAdmissionController) validateCreate(reqUID types.UID, obj runtime.Object) *admissionv1.AdmissionResponse {
	nc := obj.(*v1.NdbCluster)
	if isValid, errList := nc.HasValidSpec(); !isValid {
		// ndb does not define a valid configuration
		return requestDeniedNdbInvalid(reqUID, nc, errList)
	}

	return requestAllowed(reqUID)
}

func (nv *ndbAdmissionController) validateUpdate(
	reqUID types.UID, newObj runtime.Object, oldObj runtime.Object) *admissionv1.AdmissionResponse {

	oldNC := oldObj.(*v1.NdbCluster)
	// The Operator can handle only one update at a moment, so disallow
	// any update when the previous update has not completed yet.
	// In case of previous update failing due to an error, allow the
	// new update as it might be attempting to fix the error.
	if oldNC.Status.ProcessedGeneration != oldNC.Generation && !oldNC.HasSyncError() {
		// The previous update is still being applied, and the sync has
		// not encountered any errors so far - disallow new update.
		return requestDenied(reqUID,
			errors.NewTooManyRequestsError("previous update to the NdbCluster resource is still being applied"))
	}

	newNC := newObj.(*v1.NdbCluster)
	if isValid, errList := oldNC.IsValidSpecUpdate(newNC); !isValid {
		// new ndb does not define a valid configuration
		return requestDeniedNdbInvalid(reqUID, newNC, errList)
	}

	return requestAllowed(reqUID)
}

func (nv *ndbAdmissionController) mutate(obj runtime.Object) *jsonPatchOperations {
	nc := obj.(*v1.NdbCluster)

	var patchOps jsonPatchOperations

	// Always attach atleast one MySQL Server to the MySQL Cluster setup
	if nc.Spec.MysqlNode == nil {
		patchOps.add("/spec/mysqlNode", map[string]interface{}{
			"nodeCount":    1,
			"maxNodeCount": 1,
		})
	} else if nc.Spec.MysqlNode.NodeCount == 0 {
		// 0 MySQL Servers required - update to start atleast 1 MySQL Server
		patchOps.replace("/spec/mysqlNode/nodeCount", 1)
		if nc.Spec.MysqlNode.MaxNodeCount == 0 {
			patchOps.replace("/spec/mysqlNode/maxNodeCount", 1)
		}
	} else if nc.Spec.MysqlNode.MaxNodeCount == 0 {
		// NodeCount specified but MaxNodeCount unspecified - default it to nodeCount + 2
		patchOps.replace("/spec/mysqlNode/maxNodeCount", nc.Spec.MysqlNode.NodeCount+2)
	}

	return &patchOps
}
