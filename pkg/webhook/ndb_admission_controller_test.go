// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package webhook

import (
	"testing"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
)

func Test_ndbAdmissionController_mutate(t *testing.T) {
	type mutatorTestCases struct {
		desc          string
		ncSpec        *v1.NdbClusterSpec
		expectedPatch string
	}

	testcases := []mutatorTestCases{
		{
			desc: "mysqldSpec is nil",
			ncSpec: &v1.NdbClusterSpec{
				MysqlNode: nil,
			},
			expectedPatch: `[{"op":"add","path":"/spec/mysqlNode","value":{"maxNodeCount":1,"nodeCount":1}}]`,
		},
		{
			desc: "mysqldSpec nodeCount is 0",
			ncSpec: &v1.NdbClusterSpec{
				MysqlNode: &v1.NdbMysqldSpec{
					NodeCount: 0,
				},
			},
			expectedPatch: `[{"op":"replace","path":"/spec/mysqlNode/nodeCount","value":1},{"op":"replace","path":"/spec/mysqlNode/maxNodeCount","value":1}]`,
		},
		{
			desc: "mysqldSpec nodeCount is 1",
			ncSpec: &v1.NdbClusterSpec{
				MysqlNode: &v1.NdbMysqldSpec{
					NodeCount: 1,
				},
			},
			expectedPatch: `[{"op":"replace","path":"/spec/mysqlNode/maxNodeCount","value":3}]`,
		},
		{
			desc: "mysqldSpec nodeCount and maxNodeCount specified",
			ncSpec: &v1.NdbClusterSpec{
				MysqlNode: &v1.NdbMysqldSpec{
					NodeCount:    1,
					MaxNodeCount: 4,
				},
			},
			// No patch expected
		},
	}

	ndbAc := newNdbAdmissionController()
	nc := testutils.NewTestNdb("default", "test", 1)
	for _, tc := range testcases {
		nc.Spec = *tc.ncSpec
		originalPatch, err := ndbAc.mutate(nc).getPatch()
		if err != nil {
			t.Errorf("Testcase %q failed with error %q", tc.desc, err)
			continue
		}

		if string(originalPatch) != tc.expectedPatch {
			t.Errorf("Testcase %q failed : Expected patch `%s` but got `%s`", tc.desc, tc.expectedPatch, string(originalPatch))
		}
	}
}
