// Copyright (c) 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package v1

import (
	"testing"

	"github.com/mysql/ndb-operator/pkg/constants"
)

func TestManagementNetworkPolicyExpectedFailsClosed(t *testing.T) {
	testCases := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{name: "missing annotation", want: true},
		{
			name: "enabled",
			annotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "true",
			},
			want: true,
		},
		{
			name: "explicitly disabled",
			annotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "false",
			},
			want: false,
		},
		{
			name: "malformed annotation",
			annotations: map[string]string{
				constants.ManagementNetworkPolicyAnnotation: "malformed",
			},
			want: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ndb := &NdbCluster{}
			ndb.Annotations = tc.annotations
			if got := ndb.ManagementNetworkPolicyExpected(); got != tc.want {
				t.Fatalf("ManagementNetworkPolicyExpected()=%t, want %t", got, tc.want)
			}
		})
	}
}
