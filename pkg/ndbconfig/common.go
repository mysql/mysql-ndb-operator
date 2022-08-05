// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbconfig

import (
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
)

// getNumOfFreeAPISections returns the total number of free
// [API] sections required. This does not include the exclusive mysqld
// sections used by the MySQL Servers.
func getNumOfFreeAPISections(nc *v1alpha1.NdbCluster) int32 {
	// Reserve one API section for the operator
	numOfSectionsForOperator := int32(1)
	return nc.Spec.FreeAPISlots + numOfSectionsForOperator
}

// getNumOfSectionsRequiredForMySQLServers returns the
// number of sections required by the MySQL Servers.
func getNumOfSectionsRequiredForMySQLServers(nc *v1alpha1.NdbCluster) int32 {
	return nc.GetMySQLServerNodeCount()
}
