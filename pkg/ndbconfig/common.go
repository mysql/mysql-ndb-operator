// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbconfig

import (
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
)

// GetNumOfSectionsRequiredForMySQLServers returns the
// number of sections required by the MySQL Servers.
func GetNumOfSectionsRequiredForMySQLServers(nc *v1.NdbCluster) int32 {
	return nc.GetMySQLServerMaxNodeCount() * nc.GetMySQLServerConnectionPoolSize()
}
