// Copyright (c) 2022, 2026 Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package webhook

import (
	"encoding/json"
	"strings"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
)

func Test_ndbAdmissionController_mutate(t *testing.T) {
	type mutatorTestCases struct {
		desc                   string
		operation              admissionv1.Operation
		ncSpec                 *v1.NdbClusterSpec
		expectedCreatePatch    string
		expectedNonCreatePatch string
	}

	testcases := []mutatorTestCases{
		{
			desc: "mysqldSpec is nil",
			ncSpec: &v1.NdbClusterSpec{
				MysqlNode: nil,
			},
			expectedCreatePatch:    `[{"op":"add","path":"/metadata/annotations","value":{"mysql.oracle.com/enable-management-network-policy":"true"}},{"op":"add","path":"/spec/mysqlNode","value":{"maxNodeCount":1,"nodeCount":1}}]`,
			expectedNonCreatePatch: `[{"op":"add","path":"/spec/mysqlNode","value":{"maxNodeCount":1,"nodeCount":1}}]`,
		},
		{
			desc: "mysqldSpec nodeCount is 0",
			ncSpec: &v1.NdbClusterSpec{
				MysqlNode: &v1.NdbMysqldSpec{
					NodeCount: 0,
				},
			},
			expectedCreatePatch:    `[{"op":"add","path":"/metadata/annotations","value":{"mysql.oracle.com/enable-management-network-policy":"true"}},{"op":"replace","path":"/spec/mysqlNode/nodeCount","value":1},{"op":"replace","path":"/spec/mysqlNode/maxNodeCount","value":1}]`,
			expectedNonCreatePatch: `[{"op":"replace","path":"/spec/mysqlNode/nodeCount","value":1},{"op":"replace","path":"/spec/mysqlNode/maxNodeCount","value":1}]`,
		},
		{
			desc: "mysqldSpec nodeCount is 1",
			ncSpec: &v1.NdbClusterSpec{
				MysqlNode: &v1.NdbMysqldSpec{
					NodeCount: 1,
				},
			},
			expectedCreatePatch:    `[{"op":"add","path":"/metadata/annotations","value":{"mysql.oracle.com/enable-management-network-policy":"true"}},{"op":"replace","path":"/spec/mysqlNode/maxNodeCount","value":3}]`,
			expectedNonCreatePatch: `[{"op":"replace","path":"/spec/mysqlNode/maxNodeCount","value":3}]`,
		},
		{
			desc: "mysqldSpec nodeCount and maxNodeCount specified",
			ncSpec: &v1.NdbClusterSpec{
				MysqlNode: &v1.NdbMysqldSpec{
					NodeCount:    1,
					MaxNodeCount: 4,
				},
			},
			expectedCreatePatch: `[{"op":"add","path":"/metadata/annotations","value":{"mysql.oracle.com/enable-management-network-policy":"true"}}]`,
		},
	}
	// Verify the security annotation is create-only without changing existing defaults.
	operations := []admissionv1.Operation{
		admissionv1.Create,
		admissionv1.Update,
		admissionv1.Delete,
		admissionv1.Connect,
	}

	ndbAc := newNdbAdmissionController()
	nc := testutils.NewTestNdb("default", "test", 1)
	for _, tc := range testcases {
		nc.Spec = *tc.ncSpec

		for _, operation := range operations {
			originalPatch, err := ndbAc.mutate(nc, operation).getPatch()
			if err != nil {
				t.Errorf("Testcase %q with operation %q failed with error %q", tc.desc, operation, err)
				continue
			}

			expectedPatch := tc.expectedNonCreatePatch
			if operation == admissionv1.Create {
				expectedPatch = tc.expectedCreatePatch
			}
			if string(originalPatch) != expectedPatch {
				t.Errorf("Testcase %q with operation %q failed : Expected patch `%s` but got `%s`", tc.desc, operation, expectedPatch, string(originalPatch))
			}
		}
	}
}

func Test_ndbAdmissionController_createForcesNetworkPolicyEnabled(t *testing.T) {
	ndbAc := newNdbAdmissionController()
	nc := testutils.NewTestNdb("default", "test", 2)
	nc.Annotations = map[string]string{
		constants.ManagementNetworkPolicyAnnotation: "false",
	}

	patch, err := ndbAc.mutate(nc, admissionv1.Create).getPatch()
	if err != nil {
		t.Fatal("Failed to encode mutation patch:", err)
	}
	expected := `{"op":"add","path":"/metadata/annotations/mysql.oracle.com~1enable-management-network-policy","value":"true"}`
	if !strings.Contains(string(patch), expected) {
		t.Fatalf("Create mutation did not force management NetworkPolicy on: %s", patch)
	}
}

func Test_ndbAdmissionController_createValidationRejectsUnsafeNetworkPolicyAnnotation(t *testing.T) {
	ndbAc := newNdbAdmissionController()
	testCases := []string{"false", "malformed"}
	for _, annotationValue := range testCases {
		t.Run(annotationValue, func(t *testing.T) {
			nc := testutils.NewTestNdb("default", "test", 2)
			nc.Annotations = map[string]string{
				constants.ManagementNetworkPolicyAnnotation: annotationValue,
			}

			response := ndbAc.validateCreate(admissionv1.AdmissionRequest{}.UID, nc)
			if response.Allowed {
				t.Fatalf("Create validation accepted unsafe NetworkPolicy annotation %q", annotationValue)
			}
		})
	}
}

func Test_ndbAdmissionController_validateManagementLoadBalancer(t *testing.T) {
	ndbAc := newNdbAdmissionController()
	reqUID := admissionv1.AdmissionRequest{}.UID

	t.Run("rejects management load balancer on create", func(t *testing.T) {
		nc := testutils.NewTestNdb("default", "test", 2)
		nc.Spec.ManagementNode.EnableLoadBalancer = true

		response := ndbAc.validateCreate(reqUID, nc)
		if response.Allowed {
			t.Fatalf("Expected create request with management LoadBalancer to be denied")
		}
	})

	t.Run("rejects newly enabled management load balancer", func(t *testing.T) {
		oldNC := testutils.NewTestNdb("default", "test", 2)
		oldNC.Status.ProcessedGeneration = oldNC.Generation

		newNC := oldNC.DeepCopy()
		newNC.Spec.ManagementNode.EnableLoadBalancer = true

		response := ndbAc.validateUpdate(reqUID, newNC, oldNC)
		if response.Allowed {
			t.Fatalf("Expected update request newly enabling management LoadBalancer to be denied")
		}
	})

	t.Run("allows legacy management load balancer", func(t *testing.T) {
		oldNC := testutils.NewTestNdb("default", "test", 2)
		oldNC.Spec.ManagementNode.EnableLoadBalancer = true
		oldNC.Status.ProcessedGeneration = oldNC.Generation

		newNC := oldNC.DeepCopy()
		newNC.Annotations = map[string]string{
			constants.ManagementNetworkPolicyAnnotation: "true",
		}

		response := ndbAc.validateUpdate(reqUID, newNC, oldNC)
		if !response.Allowed {
			t.Fatalf("Expected legacy management LoadBalancer update to be allowed: %v", response.Result)
		}
	})
}

func Test_validateDoesNotTrustServiceAccountName(t *testing.T) {
	oldNC := testutils.NewTestNdb("default", "test", 2)
	oldNC.Status.ProcessedGeneration = oldNC.Generation
	newNC := oldNC.DeepCopy()
	newNC.Spec.ManagementNode.EnableLoadBalancer = true

	oldRaw, err := json.Marshal(oldNC)
	if err != nil {
		t.Fatal("Failed to encode old NdbCluster:", err)
	}
	newRaw, err := json.Marshal(newNC)
	if err != nil {
		t.Fatal("Failed to encode new NdbCluster:", err)
	}
	req := &admissionv1.AdmissionRequest{
		Operation: admissionv1.Update,
		Resource: metav1.GroupVersionResource{
			Group: "mysql.oracle.com", Version: "v1", Resource: "ndbclusters",
		},
		Object:    runtime.RawExtension{Raw: newRaw},
		OldObject: runtime.RawExtension{Raw: oldRaw},
		UserInfo: authenticationv1.UserInfo{
			Username: "system:serviceaccount:tenant:ndb-operator-evil",
		},
	}

	response := validate(req, newNdbAdmissionController())
	if response.Allowed {
		t.Fatal("A ServiceAccount name must not bypass management LoadBalancer validation")
	}
}

func Test_ndbAdmissionController_validateManagementNetworkPolicy(t *testing.T) {
	testCases := []struct {
		name           string
		oldAnnotations map[string]string
		newAnnotations map[string]string
		wantAllowed    bool
	}{
		{
			name: "allows existing cluster to opt in",
			oldAnnotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "false",
			},
			newAnnotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "true",
			},
			wantAllowed: true,
		},
		{
			name: "allows policy to remain enabled",
			oldAnnotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "true",
			},
			newAnnotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "true",
			},
			wantAllowed: true,
		},
		{
			name: "allows opted-out cluster to enable the default by removing annotation",
			oldAnnotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "false",
			},
			wantAllowed: true,
		},
		{
			name: "rejects disabling policy",
			oldAnnotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "true",
			},
			newAnnotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "false",
			},
		},
		{
			name: "rejects invalid enabled value",
			oldAnnotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "true",
			},
			newAnnotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "invalid",
			},
		},
		{
			name: "allows removing annotation because protection remains the default",
			oldAnnotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "true",
			},
			wantAllowed: true,
		},
		{
			name: "rejects explicit opt-out when protection was implicit",
			newAnnotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "false",
			},
		},
	}

	ndbAc := newNdbAdmissionController()
	reqUID := admissionv1.AdmissionRequest{}.UID

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			oldNC := testutils.NewTestNdb("default", "test", 2)
			oldNC.Status.ProcessedGeneration = oldNC.Generation
			oldNC.Annotations = tc.oldAnnotations

			newNC := oldNC.DeepCopy()
			newNC.Annotations = tc.newAnnotations

			response := ndbAc.validateUpdate(reqUID, newNC, oldNC)
			if response.Allowed != tc.wantAllowed {
				t.Fatalf("Expected update allowed=%t, got allowed=%t: %v",
					tc.wantAllowed, response.Allowed, response.Result)
			}
		})
	}
}
