package helpers

import (
	"fmt"
	"testing"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
)

type validationCase struct {
	spec       *v1alpha1.NdbSpec
	oldSpec    *v1alpha1.NdbSpec
	shouldFail bool
	explain    string
}

func nodeNumberTests(redundancy, dnc, mysqldc int32, fail bool, short string) *validationCase {
	return &validationCase{
		spec: &v1alpha1.NdbSpec{
			RedundancyLevel: redundancy,
			NodeCount:       dnc,
			Mysqld: &v1alpha1.NdbMysqldSpec{
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
		spec: &v1alpha1.NdbSpec{
			RedundancyLevel: 1,
			NodeCount:       1,
			Mysqld: &v1alpha1.NdbMysqldSpec{
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
		spec: &v1alpha1.NdbSpec{
			RedundancyLevel: redundancy,
			NodeCount:       dnc,
			Mysqld: &v1alpha1.NdbMysqldSpec{
				NodeCount: mysqldCount,
			},
		},
		oldSpec: &v1alpha1.NdbSpec{
			RedundancyLevel: oldRedundancy,
			NodeCount:       oldDnc,
			Mysqld: &v1alpha1.NdbMysqldSpec{
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
		nodeNumberTests(0, 2, 2, shouldFail, "reduncany zero, not matching node count"),
		nodeNumberTests(3, 2, 2, shouldFail, "reduncany not matching data node count"),
		nodeNumberTests(2, 145, 2, shouldFail, "too many data nodes"),
		nodeNumberTests(2, 144, 111, shouldFail, "too many nodes (including 2 mgmd nodes)"),
		nodeNumberTests(1, 144, 111, !shouldFail, "okay no of nodes (including 1 mgmd nodes)"),
		nodeNumberTests(2, 144, 2, !shouldFail, "okay"),
		nodeNumberTests(3, 9, 2, !shouldFail, "okay"),
		nodeNumberTests(2, 2, 2, !shouldFail, "all okay"),
		nodeNumberTests(1, 2, 2, !shouldFail, "2 dn and reduncany 1 is okay"),
		nodeNumberTests(2, 2, 0, !shouldFail, "okay with no mysqlds"),

		mysqldRootPasswordSecretNameTests("root-pass.123", !shouldFail, "valid name"),
		mysqldRootPasswordSecretNameTests("-root-pass123", shouldFail, "should start with an alphabet"),
		mysqldRootPasswordSecretNameTests("root-pass-", shouldFail, "should end with an alphabet"),
		mysqldRootPasswordSecretNameTests("root-pass!", shouldFail, "has invalid character"),

		ndbUpdateTests(2, 2, 2, 1, 2, 2, shouldFail, "should not update redundancy"),
		ndbUpdateTests(2, 4, 2, 2, 2, 2, shouldFail, "should not update data node count"),
		ndbUpdateTests(2, 2, 5, 2, 2, 2, !shouldFail, "allow increasing mysqld node count"),
	}

	for _, vc := range vcs {

		ndb := &v1alpha1.Ndb{
			Spec: *vc.spec,
		}

		var oldNdb *v1alpha1.Ndb
		if vc.oldSpec != nil {
			oldNdb = &v1alpha1.Ndb{
				Spec: *vc.oldSpec,
			}
		}

		if errList := IsValidConfig(ndb, oldNdb); errList != nil {
			if !vc.shouldFail {
				t.Errorf("Error \"%s\" for valid case: %s", errList.ToAggregate(), vc.explain)
			}
		} else {
			if vc.shouldFail {
				t.Errorf("Should fail with error but didn't: %s", vc.explain)
			}
		}
	}

}
