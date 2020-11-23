// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndb

import (
	"fmt"
	"testing"
)

func TestGetThreads(t *testing.T) {

	var (
		nodeID           int
		uptime           int
		status           int
		startPhase       int
		configGeneration int
	)

	ndbInfo := NewNdbConnection(dsn)
	defer ndbInfo.Free()

	{
		rows := ndbInfo.SelectAll("nodes")
		defer rows.Close()

		for rows.Next() {
			rows.Scan(&nodeID, &uptime, &status, &startPhase, &configGeneration)

			fmt.Printf("%d %d %d %d %d\n", nodeID, uptime, status, startPhase, configGeneration)
		}
	}
}
