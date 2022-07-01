// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

// Config parameters with the same ids as specified in
// storage/ndb/include/mgmapi/mgmapi_config_parameters.h
// in MySQL Cluster source code.

// System config param ids
const (
	sysCfgConfigGenerationNumber uint32 = 2
)

// Common node parameters
const (
	nodeCfgNodeId    uint32 = 3
	nodeCfgHost      uint32 = 5
	nodeCfgDatadir   uint32 = 7
	nodeCfgArbitRank uint32 = 200
)

// Data node config param ids
const (
	dbCfgNoTables             uint32 = 102 //MaxNoOfTables
	dbCfgDataMemory           uint32 = 112 //DataMemory
	dbCfgNodegroup            uint32 = 185 //Nodegroup
	dbCfgTransactionMemory    uint32 = 667 //TransactionMemory
	dbCfgNoAttributes         uint32 = 103 //MaxNoOfAttributes
	dbCfgNoOrderedIndexes     uint32 = 149 //MaxNoOfOrderedIndexes
	dbCfgNoUniqueHashIndexes  uint32 = 150 //MaxNoOfUniqueHashIndexes
	dbCfgNoOps                uint32 = 107 //MaxNoOfConcurrentOperations
	dbCfgTransBufferMem       uint32 = 111 //TransactionBufferMemory
	dbCfgIndexMem             uint32 = 113 //IndexMemory
	dbCfgRedoBuffer           uint32 = 156 //RedoBuffer
	dbCfgLongSignalBuffer     uint32 = 157 //LongMessageBuffer
	dbCfgDiskPageBufferMemory uint32 = 160 //DiskPageBufferMemory
	dbCfgSga                  uint32 = 198 //SharedGlobalMemory
	dbCfgNoRedologParts       uint32 = 632 //NoOfFragmentLogParts
)

// Mgmd config param ids
const (
	mgmCfgPort uint32 = 300
)
