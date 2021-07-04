// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import "fmt"

// getMySQLVersionString extracts the MySQL Version of
// format major.minor.build from the given versionNumber
func getMySQLVersionString(versionNumber int) string {
	if versionNumber == 0 {
		return ""
	}

	major := (versionNumber >> 16) & 0xFF
	minor := (versionNumber >> 8) & 0xFF
	build := (versionNumber >> 0) & 0xFF

	return fmt.Sprintf("%d.%d.%d", major, minor, build)
}
