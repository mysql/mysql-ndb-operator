// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package constants

import "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller"

// ClusterLabel is applied to all components of a Ndb cluster
const ClusterLabel = ndbcontroller.GroupName + "/v1alpha1"
const ClusterNodeTypeLabel = ndbcontroller.GroupName + "/nodetype"

// ClusterResourceTypeLabel is applied to all k8s resources managed by Ndb
const ClusterResourceTypeLabel = ndbcontroller.GroupName + "/resourcetype"

const DataDir = "/var/lib/ndb"

// Constants used by the MySQL Deployments
const (
	// LastAppliedConfigGeneration is the annotation key that holds the last applied config generation
	LastAppliedConfigGeneration = ndbcontroller.GroupName + "last-applied-config-generation"
	// NdbClusterInitScript is the name of the ndbcluster initialisation script
	NdbClusterInitScript = "ndbcluster-init-script.sh"
)
