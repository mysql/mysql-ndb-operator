// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package config

import (
	"flag"
)

const (
	// DefaultScriptsDir is the default value for the ScriptsDir
	// When run inside K8s cluster, the scripts are copied into
	// the container image (Refer docker/ndb-operator/Dockerfile)
	DefaultScriptsDir = "/ndb-operator-scripts"
)

var (
	// ScriptsDir stores the location of the scripts directory
	ScriptsDir string

	MasterURL  string
	Kubeconfig string

	// WatchNamespace is the namespace which needs to be watched by the operator.
	// If the argument is not set, the operator will watch for changes across
	// the K8s cluster. i.e., it will be cluster-scoped.
	WatchNamespace string
)

func InitFlags() {
	flag.StringVar(&ScriptsDir, "scripts_dir", DefaultScriptsDir,
		"The location of scripts to be deployed by the operator in the pods. Only required if out-of-cluster.")
	flag.StringVar(&Kubeconfig, "kubeconfig", "",
		"Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&MasterURL, "master", "",
		"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&WatchNamespace, "watch-namespace", "",
		"The namespace to be watched by the operator for NdbCluster resource changes. "+
			"If the option is not set, the operator will be cluster-scoped.")
}
