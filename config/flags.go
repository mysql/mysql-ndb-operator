// Copyright (c) 2020, 2025, Oracle and/or its affiliates.
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
	ClusterScoped          bool
	EnableSecurityContext  bool
	UsePlatformAssignedIDs bool
	RunAsUser              uint
	RunAsGroup             uint
	FSGroup                uint
)

func ValidateFlags() {

	if ClusterScoped && WatchNamespace != "" {
		// Ignore WatchNamespace if operator is cluster-scoped
		klog.Warning("Ignoring option 'watch-namespace' as 'cluster-scoped' is enabled")
		WatchNamespace = ""
	}

	runningInsideK8s := helpers.IsAppRunningInsideK8s()
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

	if !EnableSecurityContext {
		if UsePlatformAssignedIDs {
			klog.Warning("Ignoring option 'use-platform-assigned-ids' as 'enable-security-context' is not set")
		}
	}

}

func InitFlags() {
	flag.StringVar(&Kubeconfig, "kubeconfig", "",
		"Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&MasterURL, "master", "",
		"The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&WatchNamespace, "watch-namespace", "",
		"The namespace to be watched by the operator for NdbCluster resource changes.")
	flag.BoolVar(&ClusterScoped, "cluster-scoped", true, ""+
		"When enabled, operator looks for NdbCluster resource changes across K8s cluster.")
	flag.BoolVar(&EnableSecurityContext, "enable-security-context", false,
		"When enabled, NDB Cluster pods will be deployed with stricter security restrictions.")
	flag.BoolVar(&UsePlatformAssignedIDs, "use-platform-assigned-ids", false, ""+
		"Only applied when 'enable-security-context' is true. When enabled, it will let the platform automatically assign the UID and GID to the running NDB Cluster processes.")
	flag.UintVar(&RunAsUser, "run-as-user", 27, "UID used to run the NDB Cluster processes, when not automatically assigned by the platform")
	flag.UintVar(&RunAsGroup, "run-as-group", 27, "GID used to run the NDB Cluster processes, when not automatically assigned by the platform")
	flag.UintVar(&FSGroup, "fs-group", 27, "GID used for mounting volumes in NDB Cluster pods, when not automatically assigned by the platform")
}
