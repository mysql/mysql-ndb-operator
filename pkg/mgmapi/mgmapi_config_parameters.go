// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

// Config parameters with the same ids as specified in
// storage/ndb/include/mgmapi/mgmapi_config_parematers.h
// in MySQL Cluster source code.

// System config param ids
const (
	sysCfgConfigGenerationNumber uint32 = 2
)

// Common node parameters
const (
	nodeCfgNodeId  uint32 = 3
	nodeCfgHost    uint32 = 5
	nodeCfgDatadir uint32 = 7
)

// Data node config param ids
const (
	dbCfgDataMemory        uint32 = 112
	dbCfgTransactionMemory uint32 = 667
	dbCfgNoTables          uint32 = 102
)

// Mgmd config param ids
const (
	mgmCfgPort uint32 = 300
)
