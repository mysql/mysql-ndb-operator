// Copyright (c) 2021, 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"testing"
)

// Base 64 encoded binary config data to be used
// for tests, generated from following config :
//
// [ndbd default]
// NoOfReplicas = 2
// DataMemory=200M
// BackupDataDir= /home/mysql/datadir/ndb_data
//
// [mysqld default]
// [ndb_mgmd default]
// [tcp default]
// SendSignalId = Y
//
// [ndb_mgmd]
// NodeId = 50
// HostName = localhost
//
// [ndbd]
// NodeId = 2
// DataDir = /home/mysql/datadir/ndb_data/node2
//
// [ndbd]
// NodeId = 3
// DataDir = /home/mysql/datadir/ndb_data/node3
//
// [mysqld]
// NodeId=145
// [mysqld]
// NodeId=146
// [mysqld]
// NodeId=147
// [mysqld]
// NodeId=148
// [mysqld]
// NodeId=149
// [mysqld]
// NodeId=150
// [mysqld]
// NodeId=151
// [mysqld]
// NodeId=152
// [mysqld]
// NodeId=153
// [mysqld]
// NodeId=154
const binConfigDataBase64 = `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
EAABkgAAAAEQAAGTAAAAABAAAZYAAAAAIAABlwAAAAEAAAAAIAABmAAAAApsb2NhbGhvc3QAAAAQ
AAGZAAAANxAAAZoAAAADEAABmwAAAAAQAAGcAAAAABAAAZ0AAAAAEAABngAAAAQQAAHGACAAABAA
AccAIAAAEAAByQAAAAAQAAHKAAAAABAAAcsAAAAAEAABzAAAAAAQAAHNAAAAAAAAAAMAAAAAAAAA
BQAAAA4AAAADAAAABhAAAAEAAAAyEAAAAgAAAAEgAAADAAAAEk1DXzIwMjEwNjE0MTcwMTMyAAAA
AAAAGwAAAAMAAAABEAAAAwAAAAIgAAAHAAAAIy9ob21lL215c3FsL2RhdGFkaXIvbmRiX2RhdGEv
bm9kZTIAACAAAH0AAAAjL2hvbWUvbXlzcWwvZGF0YWRpci9uZGJfZGF0YS9ub2RlMgAAAAAABQAA
AAEAAAABEAAAAwAAAAMAAAAFAAAAAQAAAAMQAAADAAAAMgAAAAUAAAABAAAAAhAAAAMAAACRAAAA
BQAAAAEAAAACEAAAAwAAAJIAAAAFAAAAAQAAAAIQAAADAAAAkwAAAAUAAAABAAAAAhAAAAMAAACU
AAAABQAAAAEAAAACEAAAAwAAAJUAAAAFAAAAAQAAAAIQAAADAAAAlgAAAAUAAAABAAAAAhAAAAMA
AACXAAAABQAAAAEAAAACEAAAAwAAAJgAAAAFAAAAAQAAAAIQAAADAAAAmQAAAAUAAAABAAAAAhAA
AAMAAACaAAAADgAAAAQAAAAEEAABkAAAAAIQAAGRAAAAAyAAAZcAAAAKbG9jYWxob3N0AAAAEAAB
mgAAAAIAAAAQAAAABQAAAAQQAAGQAAAAMhAAAZEAAAACEAABlgAABKIgAAGXAAAACmxvY2FsaG9z
dAAAABAAAZoAAAAyAAAACQAAAAMAAAAEEAABkAAAAJEQAAGRAAAAAhAAAZoAAAACAAAACQAAAAMA
AAAEEAABkAAAAJIQAAGRAAAAAhAAAZoAAAACAAAACQAAAAMAAAAEEAABkAAAAJMQAAGRAAAAAhAA
AZoAAAACAAAACQAAAAMAAAAEEAABkAAAAJQQAAGRAAAAAhAAAZoAAAACAAAACQAAAAMAAAAEEAAB
kAAAAJUQAAGRAAAAAhAAAZoAAAACAAAACQAAAAMAAAAEEAABkAAAAJYQAAGRAAAAAhAAAZoAAAAC
AAAACQAAAAMAAAAEEAABkAAAAJcQAAGRAAAAAhAAAZoAAAACAAAACQAAAAMAAAAEEAABkAAAAJgQ
AAGRAAAAAhAAAZoAAAACAAAACQAAAAMAAAAEEAABkAAAAJkQAAGRAAAAAhAAAZoAAAACAAAACQAA
AAMAAAAEEAABkAAAAJoQAAGRAAAAAhAAAZoAAAACBW88Lg==`

func verifyValue(t *testing.T, id int, actualValue configValue, expectedValue interface{}) {
	var match bool
	switch expectedValue.(type) {
	case uint32:
		match = actualValue.(uint32) == expectedValue.(uint32)
	case uint64:
		match = actualValue.(uint64) == expectedValue.(uint64)
	case string:
		match = actualValue.(string) == expectedValue.(string)
	default:
		panic("expectedValue has an unsupported type")
	}

	if !match {
		t.Errorf("TC#%d : readConfig returned unexpected value : %v, expected : %v",
			id, actualValue, expectedValue)
	}
}

func TestConfigReader_ReadConfig(t *testing.T) {
	cr, err := getNewConfigReader(binConfigDataBase64)
	if err != nil {
		t.Fatalf("getNewConfigReader failed: %v", err)
	}

	for id, tc := range []*struct {
		sectionTypeFilter     cfgSectionType
		fromNodeId, configKey uint32
		expectedValue         interface{}
	}{
		{
			// extract a config from system section
			sectionTypeFilter: cfgSectionTypeSystem,
			configKey:         sysCfgConfigGenerationNumber,
			expectedValue:     uint32(1),
		},
		{
			// extract a default mgmd config set by itself
			sectionTypeFilter: cfgSectionTypeMGM,
			configKey:         mgmCfgPort,
			expectedValue:     uint32(1186),
		},
		{
			// extract a mgm config set by config file
			sectionTypeFilter: cfgSectionTypeMGM,
			configKey:         nodeCfgHost,
			expectedValue:     "localhost",
		},
		{
			// extract a default ndbd config set by itself
			sectionTypeFilter: cfgSectionTypeNDB,
			configKey:         dbCfgNoTables,
			expectedValue:     uint32(128),
		},
		{
			// extract a default ndbd config set by config file default section
			sectionTypeFilter: cfgSectionTypeNDB,
			configKey:         dbCfgDataMemory,
			expectedValue:     uint64(200 * 1024 * 1024),
		},
		{
			// extract a ndbd config set by config file's node section
			// this should return the data dir of the first data node it parses
			sectionTypeFilter: cfgSectionTypeNDB,
			configKey:         nodeCfgDatadir,
			expectedValue:     "/home/mysql/datadir/ndb_data/node2",
		},
		{
			// extract a ndbd config set by config file's node section
			// this should return the data dir of the node with id 3
			sectionTypeFilter: cfgSectionTypeNDB,
			configKey:         nodeCfgDatadir,
			fromNodeId:        uint32(3),
			expectedValue:     "/home/mysql/datadir/ndb_data/node3",
		},
	} {
		value, err := cr.readConfig(tc.sectionTypeFilter, tc.fromNodeId, tc.configKey)
		if err != nil {
			t.Fatalf("TC#%d: readConfig failed: %v", id, err)
		}
		verifyValue(t, id, value, tc.expectedValue)
	}
}

func encodeConfigWords(magic string, words ...uint32) string {
	data := []byte(magic)
	for _, word := range words {
		encodedWord := make([]byte, 4)
		binary.BigEndian.PutUint32(encodedWord, word)
		data = append(data, encodedWord...)
	}
	return base64.StdEncoding.EncodeToString(data)
}

func TestConfigReaderRejectsMalformedConfig(t *testing.T) {
	const invalidEntryType = 15
	stringKey := uint32(CfgEntryTypeString<<v2TypeShift) | sysCfgConfigGenerationNumber
	intKey := uint32(CfgEntryTypeInt<<v2TypeShift) | sysCfgConfigGenerationNumber
	nodeIdInt64Key := uint32(CfgEntryTypeInt64<<v2TypeShift) | nodeCfgNodeId

	testCases := []struct {
		name              string
		encodedData       string
		sectionTypeFilter cfgSectionType
		fromNodeId        uint32
		configKey         uint32
	}{
		{
			name:              "invalid base64",
			encodedData:       "%",
			sectionTypeFilter: cfgSectionTypeSystem,
			configKey:         sysCfgConfigGenerationNumber,
		},
		{
			name:              "truncated header",
			encodedData:       "AAAA",
			sectionTypeFilter: cfgSectionTypeSystem,
			configKey:         sysCfgConfigGenerationNumber,
		},
		{
			name: "unexpected magic",
			encodedData: encodeConfigWords(
				"BADCONF2", 9, 2, 0, 0, 0, 0, 0),
			sectionTypeFilter: cfgSectionTypeSystem,
			configKey:         sysCfgConfigGenerationNumber,
		},
		{
			name: "declared length mismatch",
			encodedData: encodeConfigWords(
				configMagic, 10, 2, 0, 0, 0, 0, 0),
			sectionTypeFilter: cfgSectionTypeSystem,
			configKey:         sysCfgConfigGenerationNumber,
		},
		{
			name: "short section",
			encodedData: encodeConfigWords(
				configMagic, 15, 2, 1, 0, 0, 0, 0,
				2, 0, uint32(cfgSectionTypeNDB),
				3, 0, uint32(cfgSectionTypeSystem)),
			sectionTypeFilter: cfgSectionTypeSystem,
			configKey:         sysCfgConfigGenerationNumber,
		},
		{
			name: "section exceeds data",
			encodedData: encodeConfigWords(
				configMagic, 12, 2, 0, 0, 0, 0, 0,
				100, 0, uint32(cfgSectionTypeSystem)),
			sectionTypeFilter: cfgSectionTypeSystem,
			configKey:         sysCfgConfigGenerationNumber,
		},
		{
			name: "unsupported entry type",
			encodedData: encodeConfigWords(
				configMagic, 13, 2, 0, 0, 0, 0, 0,
				4, 1, uint32(cfgSectionTypeSystem),
				uint32(invalidEntryType<<v2TypeShift)|sysCfgConfigGenerationNumber),
			sectionTypeFilter: cfgSectionTypeSystem,
			configKey:         sysCfgConfigGenerationNumber,
		},
		{
			name: "entry exceeds section",
			encodedData: encodeConfigWords(
				configMagic, 13, 2, 0, 0, 0, 0, 0,
				4, 1, uint32(cfgSectionTypeSystem), intKey),
			sectionTypeFilter: cfgSectionTypeSystem,
			configKey:         sysCfgConfigGenerationNumber,
		},
		{
			name: "zero-length string",
			encodedData: encodeConfigWords(
				configMagic, 14, 2, 0, 0, 0, 0, 0,
				5, 1, uint32(cfgSectionTypeSystem), stringKey, 0),
			sectionTypeFilter: cfgSectionTypeSystem,
			configKey:         sysCfgConfigGenerationNumber,
		},
		{
			name: "unterminated string",
			encodedData: encodeConfigWords(
				configMagic, 15, 2, 0, 0, 0, 0, 0,
				6, 1, uint32(cfgSectionTypeSystem), stringKey, 4, 0x41424344),
			sectionTypeFilter: cfgSectionTypeSystem,
			configKey:         sysCfgConfigGenerationNumber,
		},
		{
			name: "invalid node id type",
			encodedData: encodeConfigWords(
				configMagic, 18, 2, 0, 1, 0, 0, 0,
				3, 0, uint32(cfgSectionTypeSystem),
				6, 1, uint32(cfgSectionTypeNDB), nodeIdInt64Key, 0, 1),
			sectionTypeFilter: cfgSectionTypeNDB,
			fromNodeId:        1,
			configKey:         dbCfgNodegroup,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Peer-controlled config data must always fail through the error path.
			if value, err := readConfigFromBase64EncodedData(
				tc.encodedData, tc.sectionTypeFilter, tc.fromNodeId, tc.configKey); err == nil {
				t.Fatalf("expected malformed config to fail, got %v", value)
			}
		})
	}
}

func newGetConfigReply(encodedData string) []byte {
	return []byte(fmt.Sprintf(`get config reply
result: Ok
Content-Length: %d
Content-Type: ndbconfig/octet-stream
Content-Transfer-Encoding: base64

%s`, len(encodedData), encodedData))
}

func TestMgmClientImpl_getConfig_rejectsMalformedContent(t *testing.T) {
	mgmServer, mci := newFakeMgmServerAndClient(t)
	defer mci.Disconnect()
	defer mgmServer.disconnect()

	// Exercise malformed content through the complete management reply path.
	mgmServer.run(newGetConfigReply("AAAA"))

	if _, err := mci.getConfig(
		1, cfgSectionTypeSystem, sysCfgConfigGenerationNumber, true); err == nil {
		t.Fatal("expected malformed get config content to fail")
	}
}

func FuzzConfigReaderDoesNotPanic(f *testing.F) {
	validData, err := base64.StdEncoding.DecodeString(binConfigDataBase64)
	if err != nil {
		f.Fatalf("failed to decode valid config seed: %v", err)
	}
	f.Add([]byte{})
	f.Add(validData)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Keep arbitrary management replies on the parser's error-return path.
		// No panic should occur
		_, _ = readConfigFromBase64EncodedData(
			base64.StdEncoding.EncodeToString(data),
			cfgSectionTypeSystem, 0, sysCfgConfigGenerationNumber)
	})
}
