package helpers

import (
	"fmt"
	"testing"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
)

type validationCase struct {
	spec       *v1alpha1.NdbSpec
	shouldFail bool
	explain    string
}

func nodeNumberTests(reduncany, dnc, mysqldc int, fail bool, short string) validationCase {
	return validationCase{
		spec: &v1alpha1.NdbSpec{
			RedundancyLevel: IntToInt32Ptr(int(reduncany)),
			NodeCount:       IntToInt32Ptr(int(dnc)),
			ContainerImage:  "mysql/mysql-cluster:8.0.22",
			Mysqld: &v1alpha1.NdbMysqldSpec{
				NodeCount: int32(mysqldc),
			},
		},
		shouldFail: fail,
		explain: fmt.Sprintf("%3d reduncany, %3d data nodes, %3d mysql nodes - %s",
			reduncany, dnc, mysqldc, short),
	}
}

func Test_InvalidValues(t *testing.T) {

	ndb := NewTestNdb("ns", "test", 2)
	if err := IsValidConfig(ndb); err != nil {
		t.Errorf("2 node cluster is wrongly marked invalid with error %s", err)
	}

	shouldFail := true
	vcs := []validationCase{
		nodeNumberTests(0, 0, 0, shouldFail, "all zero"),
		nodeNumberTests(0, 2, 2, shouldFail, "reduncany zero, not matching node count"),
		nodeNumberTests(5, 0, 0, shouldFail, "too high reduncany"),
		nodeNumberTests(3, 2, 2, shouldFail, "reduncany not matching data node count"),
		nodeNumberTests(2, 145, 2, shouldFail, "too many data nodes"),
		nodeNumberTests(2, 144, 111, shouldFail, "too many nodes (including 2 mgmd nodes)"),
		nodeNumberTests(1, 144, 111, !shouldFail, "okay no of nodes (including 1 mgmd nodes)"),
		nodeNumberTests(2, 144, 2, !shouldFail, "okay"),
		nodeNumberTests(3, 9, 2, !shouldFail, "okay"),
		nodeNumberTests(2, 2, 2, !shouldFail, "all okay"),
		nodeNumberTests(1, 2, 2, !shouldFail, "2 dn and reduncany 1 is okay"),
		nodeNumberTests(2, 2, 0, !shouldFail, "okay with no mysqlds"),
	}

	for _, vc := range vcs {

		vc.spec.DeepCopyInto(&ndb.Spec)

		if err := IsValidConfig(ndb); err != nil {
			if !IsInvalidConfiguration(err) {
				t.Errorf("Wrong error type returned %s for:  %s", err, vc.explain)
			}
			if !vc.shouldFail {
				t.Errorf("Error %s for valid case:           %s", err, vc.explain)
			}
		} else {
			if vc.shouldFail {
				t.Errorf("Should fail with error but didn't: %s", vc.explain)
			}
		}
	}

}
