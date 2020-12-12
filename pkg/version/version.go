// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package version

var buildVersion = "0.0.0"
var buildTime = ""

// GetBuildVersion returns the NDB Operator build version
func GetBuildVersion() string {
	return buildVersion
}

// GetBuildTime returns the Ndb operator build time
func GetBuildTime() string {
	return buildTime
}
