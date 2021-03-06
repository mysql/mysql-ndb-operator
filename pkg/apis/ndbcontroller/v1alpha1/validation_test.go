// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package v1alpha1

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

type validationCase struct {
	spec       *NdbClusterSpec
	oldSpec    *NdbClusterSpec
	shouldFail bool
	explain    string
}

func nodeNumberTests(redundancy, dnc, mysqldc int32, fail bool, short string) *validationCase {
	return &validationCase{
		spec: &NdbClusterSpec{
			RedundancyLevel: redundancy,
			NodeCount:       dnc,
			Mysqld: &NdbMysqldSpec{
				NodeCount: mysqldc,
			},
		},
		shouldFail: fail,
		explain: fmt.Sprintf("%3d redundancy, %3d data nodes, %3d mysql nodes - %s",
			redundancy, dnc, mysqldc, short),
	}
}

func mysqldRootPasswordSecretNameTests(secretName string, fail bool, short string) *validationCase {
	return &validationCase{
		spec: &NdbClusterSpec{
			RedundancyLevel: 1,
			NodeCount:       1,
			Mysqld: &NdbMysqldSpec{
				NodeCount:              1,
				RootPasswordSecretName: secretName,
			},
		},
		shouldFail: fail,
		explain:    fmt.Sprintf("RootPasswordSecretName : '%s'- %s", secretName, short),
	}
}

func ndbUpdateTests(redundancy, dnc, mysqldCount,
	oldRedundancy, oldDnc, oldMysqldCount int32,
	fail bool, short string) *validationCase {
	return &validationCase{
		spec: &NdbClusterSpec{
			RedundancyLevel: redundancy,
			NodeCount:       dnc,
			Mysqld: &NdbMysqldSpec{
				NodeCount: mysqldCount,
			},
		},
		oldSpec: &NdbClusterSpec{
			RedundancyLevel: oldRedundancy,
			NodeCount:       oldDnc,
			Mysqld: &NdbMysqldSpec{
				NodeCount: oldMysqldCount,
			},
		},
		shouldFail: fail,
		explain:    short,
	}
}

func ndbUpdateNdbPodSpecTests(
	oldNdbClusterSpec func(defaultSpec *NdbClusterSpec),
	newNdbClusterSpec func(defaultSpec *NdbClusterSpec),
	fail bool, short string) *validationCase {
	vc := &validationCase{
		spec: &NdbClusterSpec{
			RedundancyLevel: 2,
			NodeCount:       2,
		},
		oldSpec: &NdbClusterSpec{
			RedundancyLevel: 2,
			NodeCount:       2,
		},
		shouldFail: fail,
		explain:    short,
	}
	oldNdbClusterSpec(vc.oldSpec)
	newNdbClusterSpec(vc.spec)
	return vc
}

func Test_Validation(t *testing.T) {

	shouldFail := true
	vcs := []*validationCase{
		nodeNumberTests(0, 0, 0, shouldFail, "all zero"),
		nodeNumberTests(0, 2, 2, shouldFail, "redundancy zero, not matching node count"),
		nodeNumberTests(3, 2, 2, shouldFail, "redundancy not matching data node count"),
		nodeNumberTests(2, 145, 2, shouldFail, "too many data nodes"),
		nodeNumberTests(2, 144, 111, shouldFail, "too many nodes (including 2 mgmd nodes)"),
		nodeNumberTests(1, 144, 111, !shouldFail, "okay no of nodes (including 1 mgmd nodes)"),
		nodeNumberTests(2, 144, 2, !shouldFail, "okay"),
		nodeNumberTests(3, 9, 2, !shouldFail, "okay"),
		nodeNumberTests(2, 2, 2, !shouldFail, "all okay"),
		nodeNumberTests(1, 2, 2, !shouldFail, "2 dn and redundancy 1 is okay"),
		nodeNumberTests(2, 2, 0, !shouldFail, "okay with no mysqlds"),

		mysqldRootPasswordSecretNameTests("root-pass.123", !shouldFail, "valid name"),
		mysqldRootPasswordSecretNameTests("-root-pass123", shouldFail, "should start with an alphabet"),
		mysqldRootPasswordSecretNameTests("root-pass-", shouldFail, "should end with an alphabet"),
		mysqldRootPasswordSecretNameTests("root-pass!", shouldFail, "has invalid character"),

		ndbUpdateTests(2, 2, 2, 1, 2, 2, shouldFail, "should not update redundancy"),
		ndbUpdateTests(2, 4, 2, 2, 2, 2, shouldFail, "should not update data node count"),
		ndbUpdateTests(2, 2, 5, 2, 2, 2, !shouldFail, "allow increasing mysqld node count"),
		ndbUpdateTests(1, 2, 5, 1, 2, 2, shouldFail, "update spec with replica = 1"),

		// TODO: Test currently fails as updating NdbPodSpec is denied - fix
		//       it once NDB Operator starts supporting NdbPodSpec update
		ndbUpdateNdbPodSpecTests(func(defaultSpec *NdbClusterSpec) {
			defaultSpec.DataNodePodSpec = nil
		}, func(defaultSpec *NdbClusterSpec) {
			defaultSpec.DataNodePodSpec = &NdbPodSpec{
				SchedulerName: "custom-scheduler",
			}
		}, shouldFail, "allow update to non-resource fields"),

		// TODO: Test currently fails as updating NdbPodSpec is denied - fix
		//       it once NDB Operator starts supporting NdbPodSpec update
		ndbUpdateNdbPodSpecTests(func(defaultSpec *NdbClusterSpec) {
			defaultSpec.DataNodePodSpec = &NdbPodSpec{
				NodeSelector: map[string]string{
					"key1": "value2",
				},
			}
		}, func(defaultSpec *NdbClusterSpec) {
			defaultSpec.DataNodePodSpec = nil
		}, shouldFail, "allow update to non-resource fields(2)"),

		ndbUpdateNdbPodSpecTests(func(defaultSpec *NdbClusterSpec) {
			defaultSpec.DataNodePodSpec = nil
		}, func(defaultSpec *NdbClusterSpec) {
			defaultSpec.DataNodePodSpec = &NdbPodSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceStorage: resource.MustParse("100"),
					},
				},
			}
		}, shouldFail, "should not update data node resource requirements"),

		ndbUpdateNdbPodSpecTests(func(defaultSpec *NdbClusterSpec) {
			defaultSpec.DataNodePodSpec = &NdbPodSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceStorage: resource.MustParse("100"),
					},
				},
			}
		}, func(defaultSpec *NdbClusterSpec) {
			defaultSpec.DataNodePodSpec = nil
		}, shouldFail, "should not update data node resource requirements to nil"),

		ndbUpdateNdbPodSpecTests(func(defaultSpec *NdbClusterSpec) {
			defaultSpec.DataNodePodSpec = &NdbPodSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceStorage: resource.MustParse("100"),
					},
				},
			}
		}, func(defaultSpec *NdbClusterSpec) {
			defaultSpec.DataNodePodSpec = &NdbPodSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceMemory: resource.MustParse("100"),
					},
				},
			}
		}, shouldFail, "should not update data node resource requirements definition"),

		// TODO: Test currently fails as updating NdbPodSpec is denied - fix
		//       it once NDB Operator starts supporting NdbPodSpec update
		ndbUpdateNdbPodSpecTests(func(defaultSpec *NdbClusterSpec) {
			defaultSpec.ManagementNodePodSpec = &NdbPodSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceStorage: resource.MustParse("100"),
					},
				},
			}
		}, func(defaultSpec *NdbClusterSpec) {
			defaultSpec.ManagementNodePodSpec = &NdbPodSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourceStorage: resource.MustParse("100"),
					},
				},
				SchedulerName: "custom-scheduler",
			}
		}, shouldFail, "allow update if Resources did not change"),

		ndbUpdateNdbPodSpecTests(func(defaultSpec *NdbClusterSpec) {
			defaultSpec.Mysqld = &NdbMysqldSpec{
				NodeCount: 1,
				PodSpec: &NdbPodSpec{
					Resources: &corev1.ResourceRequirements{
						Limits: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceStorage: resource.MustParse("100"),
						},
					},
				},
			}
		}, func(defaultSpec *NdbClusterSpec) {
			defaultSpec.Mysqld = &NdbMysqldSpec{
				NodeCount: 10,
				PodSpec: &NdbPodSpec{
					Resources: &corev1.ResourceRequirements{
						Limits: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceStorage: resource.MustParse("100"),
						},
					},
				},
			}
		}, !shouldFail, "allow update if Resources did not change (2)"),
	}

	for _, vc := range vcs {

		ndb := &NdbCluster{
			Spec: *vc.spec,
		}

		var isValid bool
		var errList field.ErrorList
		if vc.oldSpec != nil {
			oldNdb := &NdbCluster{
				Spec: *vc.oldSpec,
			}
			isValid, errList = oldNdb.IsValidSpecUpdate(ndb)
		} else {
			isValid, errList = ndb.HasValidSpec()
		}

		if vc.shouldFail && isValid {
			t.Errorf("Should fail with error but didn't: %s", vc.explain)
		} else if !vc.shouldFail && !isValid {
			t.Errorf("Should pass but failed : %s\nErrors : %s", vc.explain, errList.ToAggregate())
		}
	}

}
