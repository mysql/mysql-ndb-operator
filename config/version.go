// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package config

var version string
var gitCommit string

// GetBuildVersion returns the NDB Operator build version
func GetBuildVersion() string {
	if version == "" || gitCommit == "" {
		panic("version or git commit was not set during build")
	}
	return version + "-" + gitCommit
}
