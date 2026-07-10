// Copyright (c) 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"testing"
	"time"

	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func findStatusCondition(
	t *testing.T, status *v1.NdbClusterStatus, conditionType v1.NdbClusterConditionType,
) v1.NdbClusterCondition {
	t.Helper()
	for _, condition := range status.Conditions {
		if condition.Type == conditionType {
			return condition
		}
	}
	t.Fatalf("Condition %q was not found in status: %#v", conditionType, status.Conditions)
	return v1.NdbClusterCondition{}
}

func TestCalculateManagementAccessCondition(t *testing.T) {
	testCases := []struct {
		name         string
		annotations  map[string]string
		loadBalancer bool
		wantStatus   corev1.ConditionStatus
		wantReason   string
	}{
		{
			name:       "default protection",
			wantStatus: corev1.ConditionTrue,
			wantReason: v1.NdbClusterManagementAccessProtectedReason,
		},
		{
			name: "explicit legacy opt-out",
			annotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "false",
			},
			wantStatus: corev1.ConditionFalse,
			wantReason: v1.NdbClusterManagementNetworkPolicyDisabledReason,
		},
		{
			name:         "legacy management load balancer",
			loadBalancer: true,
			wantStatus:   corev1.ConditionFalse,
			wantReason:   v1.NdbClusterManagementLoadBalancerEnabledReason,
		},
		{
			name: "load balancer takes precedence over policy opt-out",
			annotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "false",
			},
			loadBalancer: true,
			wantStatus:   corev1.ConditionFalse,
			wantReason:   v1.NdbClusterManagementLoadBalancerEnabledReason,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ndb := testutils.NewTestNdb("default", "test", 2)
			ndb.Annotations = tc.annotations
			ndb.Spec.ManagementNode.EnableLoadBalancer = tc.loadBalancer
			sc := &SyncContext{ndb: ndb, syncSuccess: true}

			status := sc.calculateNdbClusterStatus()
			condition := findStatusCondition(t, status, v1.NdbClusterManagementAccessProtected)
			if condition.Status != tc.wantStatus || condition.Reason != tc.wantReason {
				t.Fatalf("Unexpected management access condition: %#v", condition)
			}
		})
	}
}

func TestCalculateStatusPreservesUnchangedConditionTransitionTimes(t *testing.T) {
	ndb := testutils.NewTestNdb("default", "test", 2)
	sc := &SyncContext{ndb: ndb, syncSuccess: true}
	originalStatus := sc.calculateNdbClusterStatus()
	transitionTime := metav1.NewTime(time.Unix(123, 0))
	for index := range originalStatus.Conditions {
		originalStatus.Conditions[index].LastTransitionTime = transitionTime
	}
	ndb.Status = *originalStatus

	recalculatedStatus := sc.calculateNdbClusterStatus()
	for _, condition := range recalculatedStatus.Conditions {
		if !condition.LastTransitionTime.Equal(&transitionTime) {
			t.Fatalf("Condition %q changed transition time without changing state: %#v",
				condition.Type, condition)
		}
	}
}

func TestPreserveConditionTransitionTimeUntilStatusChanges(t *testing.T) {
	transitionTime := metav1.NewTime(time.Unix(123, 0))
	oldCondition := v1.NdbClusterCondition{
		Type:               v1.NdbClusterUpToDate,
		Status:             corev1.ConditionFalse,
		Reason:             "old reason",
		LastTransitionTime: transitionTime,
	}
	newConditions := []v1.NdbClusterCondition{
		{
			Type:               v1.NdbClusterUpToDate,
			Status:             corev1.ConditionFalse,
			Reason:             "new reason",
			LastTransitionTime: metav1.Now(),
		},
	}

	preserveConditionTransitionTimes([]v1.NdbClusterCondition{oldCondition}, newConditions)
	if !newConditions[0].LastTransitionTime.Equal(&transitionTime) {
		t.Fatalf("Condition transition time changed while status remained False: %#v", newConditions[0])
	}

	newConditions[0].Status = corev1.ConditionTrue
	newTransitionTime := metav1.Now()
	newConditions[0].LastTransitionTime = newTransitionTime
	preserveConditionTransitionTimes([]v1.NdbClusterCondition{oldCondition}, newConditions)
	if !newConditions[0].LastTransitionTime.Equal(&newTransitionTime) {
		t.Fatalf("Condition transition time was preserved after status changed: %#v", newConditions[0])
	}
}

func TestStatusEqualComparesConditionsByType(t *testing.T) {
	conditionA := v1.NdbClusterCondition{
		Type:   v1.NdbClusterUpToDate,
		Status: corev1.ConditionTrue,
		Reason: "a",
	}
	conditionB := v1.NdbClusterCondition{
		Type:   v1.NdbClusterManagementAccessProtected,
		Status: corev1.ConditionTrue,
		Reason: "b",
	}
	oldStatus := &v1.NdbClusterStatus{Conditions: []v1.NdbClusterCondition{conditionA, conditionB}}
	newStatus := &v1.NdbClusterStatus{Conditions: []v1.NdbClusterCondition{conditionB, conditionA}}
	if !statusEqual(oldStatus, newStatus) {
		t.Fatal("Equivalent conditions in a different order must compare equal")
	}

	newStatus.Conditions[0].Message = "changed"
	if statusEqual(oldStatus, newStatus) {
		t.Fatal("A changed condition message must make statuses unequal")
	}
}
