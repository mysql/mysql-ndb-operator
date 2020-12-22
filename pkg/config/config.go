// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// Package config holds any parsed config from the command line
package config

const (
	// DefaultScriptsDir is the default value for the ScriptsDir
	// When run inside K8s cluster, the scripts are copied into
	// the container image (Refer docker/ndb-operator/Dockerfile)
	DefaultScriptsDir = "/ndb-operator-scripts"
)

var (
	// ScriptsDir stores the location of the scripts directory
	ScriptsDir string
)
