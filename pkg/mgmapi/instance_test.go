// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import (
	"testing"
)

func TestGetParentNameAndOrdinal(t *testing.T) {
	testCases := []struct {
		hostname string
		name     string
		ordinal  int
	}{
		{
			hostname: "host-99.example-domain.de",
			name:     "host",
			ordinal:  99,
		}, {
			hostname: "host-with-dashes-99.mysql.oracle.com",
			name:     "host-with-dashes",
			ordinal:  99,
		}, {
			hostname: "host_with_no_dashes",
			name:     "",
			ordinal:  -1,
		}, {
			hostname: "host-string_instead_of_ordinal",
			name:     "",
			ordinal:  -1,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.hostname, func(t *testing.T) {
			name, ordinal := GetParentNameAndOrdinal(tt.hostname)
			if name != tt.name || ordinal != tt.ordinal {
				t.Errorf("getParentNameAndOrdinal(%q) => (%q, %d) expected (%q, %d)",
					tt.hostname, name, ordinal, tt.name, tt.ordinal)
			}
		})
	}
}
