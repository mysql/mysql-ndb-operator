// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbconfig

import (
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
)

// getNumOfRequiredAPISections returns the number of [API] sections
// required by the MySQL Servers and the operator. This does not
// include the FreeApiSlots declared in the NdbCluster.Spec.
func getNumOfRequiredAPISections(nc *v1alpha1.NdbCluster, oldConfigSummary *ConfigSummary) int32 {
	// Reserve one API section for the operator
	numOfSectionsForOperator := int32(1)

	requiredNumOfSlotsForMySQLServer := nc.GetMySQLServerNodeCount()
	if requiredNumOfSlotsForMySQLServer == 0 {
		// No sections required for MySQL Servers
		return numOfSectionsForOperator
	}

	if oldConfigSummary != nil {
		// An update has been applied to the Ndb resource.
		// If the new update has requested for more MySQL Servers,
		// increase the slots if required, but if a scale down has
		// been requested, do not decrease the slots. This is to
		// avoid a potential issue where the remaining MySQL
		// Servers after the scale down might have mismatching NodeIds
		// with the ones specified in the config file causing the
		// setup to go into a degraded state.
		// TODO: Revisit this when the MySQL Servers are deployed as a SfSet
		existingNumOfSlotsForMySQLServer := oldConfigSummary.NumOfMySQLServers
		if requiredNumOfSlotsForMySQLServer < existingNumOfSlotsForMySQLServer {
			// Scale down requested - retain the existingNumOfSlotsForMySQLServer
			requiredNumOfSlotsForMySQLServer = existingNumOfSlotsForMySQLServer
		}
	}

	return requiredNumOfSlotsForMySQLServer + numOfSectionsForOperator
}
