// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package v1alpha1

import (
	"fmt"
	"testing"

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

func Test_InvalidValues(t *testing.T) {

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
			t.Errorf("Error %q for valid case: %s", errList.ToAggregate(), vc.explain)
		}
	}

}
