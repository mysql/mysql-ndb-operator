// Copyright (c) 2020, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package config

import (
	"flag"

	"github.com/mysql/ndb-operator/pkg/helpers"
	klog "k8s.io/klog/v2"
)

var (
	MasterURL  string
	Kubeconfig string

	// WatchNamespace is the namespace which needs to be watched by the operator.
	WatchNamespace string
	// ClusterScoped if set, operator will watch the entire cluster
	ClusterScoped bool
)

func ValidateFlags() {

	runningInsideK8s := helpers.IsAppRunningInsideK8s()
	if runningInsideK8s && WatchNamespace != "" {
		// Ignore WatchNamespace if operator is running inside cluster
		klog.Warning("Ignoring option 'watch-namespace' as operator is running inside K8s Cluster")
		WatchNamespace = ""
	} else if ClusterScoped && WatchNamespace != "" {
		// Ignore WatchNamespace if operator is cluster-scoped
		klog.Warning("Ignoring option 'watch-namespace' as 'cluster-scoped' is enabled")
		WatchNamespace = ""
	}

	if !runningInsideK8s {
		if Kubeconfig == "" && MasterURL == "" {
			// Operator is running out of K8s Cluster but kubeconfig/masterURL are not specified.
			klog.Fatal("Ndb operator cannot connect to the Kubernetes Server.\n" +
				"Please specify kubeconfig or masterURL.")
		}

		// WatchNamespace is required for operator running in namespace-scoped mode out-of-cluster.
		if !ClusterScoped && WatchNamespace == "" {
			klog.Fatal("For operator running in namespace-scoped mode out-of-cluster, " +
				"'-watch-namespace' argument needs to be provided.")
		}
	}

}

func InitFlags() {
	flag.StringVar(&Kubeconfig, "kubeconfig", "",
		"Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&MasterURL, "master", "",
		"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&WatchNamespace, "watch-namespace", "",
		"The namespace to be watched by the operator for NdbCluster resource changes."+
			"Only required if out-of-cluster.")
	flag.BoolVar(&ClusterScoped, "cluster-scoped", true, ""+
		"When enabled, operator looks for NdbCluster resource changes across K8s cluster.")
}
