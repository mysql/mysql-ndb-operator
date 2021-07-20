// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import (
	"encoding/base64"
	"encoding/binary"
	"github.com/mysql/ndb-operator/config/debug"
)

type configValue interface{}

// configReader helps in extracting config information from a given binary config data
type configReader struct {
	// raw data and offset
	data   []byte
	offset uint32

	// extracted information
	totalLengthInWords, version                         uint32
	numOfDefaultSections, numOfNodes, numOfCommSections uint32
	value                                               configValue
}

// getNewConfigReader creates a new configReader
// for the given base 64 encoded config data
func getNewConfigReader(base64EncodedData string) *configReader {
	// decode the string
	decodedData, err := base64.StdEncoding.DecodeString(base64EncodedData)
	if err != nil {
		debug.Panic("failed to decode config string : " + err.Error())
		return nil
	}

	return &configReader{data: decodedData}
}

// readUint32 reads an uint32 from the data at the current offset
func (cr *configReader) readUint32() uint32 {
	v := binary.BigEndian.Uint32(cr.data[cr.offset : cr.offset+4])
	cr.offset += 4
	return v
}

// readString reads a string from the data at the current offset
func (cr *configReader) readString() string {
	// Extract the length of the string
	strLen := cr.readUint32()
	// Extract the string.
	// Null character is present at the end and should not be read.
	s := string(cr.data[cr.offset : cr.offset+strLen-1])
	// update offset
	strLen = strLen + ((4 - (strLen & 3)) & 3)
	cr.offset += strLen
	return s
}

// constants required for extracting the entry key and values
const (
	v2TypeShift = 28
	v2TypeMask  = 15
	v2KeyShift  = 0
	v2KeyMask   = 0x0FFFFFFF
)

// Config entry value types used by readEntry
const (
	// CfgEntryTypeInvalid = 0
	CfgEntryTypeInt    = 1
	CfgEntryTypeString = 2
	// CfgEntryTypeSection = 3
	CfgEntryTypeInt64 = 4
)

// readEntry reads an entry from the data at the current offset
func (cr *configReader) readEntry() (uint32, configValue) {

	// Extract key and key type
	keyAndType := cr.readUint32()
	key := (keyAndType >> v2KeyShift) & v2KeyMask
	keyType := (keyAndType >> v2TypeShift) & v2TypeMask

	// Read the value based on the key type
	var value interface{}
	switch keyType {
	case CfgEntryTypeInt:
		{
			value = cr.readUint32()
		}
	case CfgEntryTypeInt64:
		{
			high := cr.readUint32()
			low := cr.readUint32()
			value = (uint64(high) << 32) + uint64(low)
		}
	case CfgEntryTypeString:
		{
			value = cr.readString()
		}
	default:
		debug.Panic("unsupported keyType in readEntry()")
	}

	return key, value
}

// cfgSectionType is the section types used by the config section
type cfgSectionType int

const (
	// cfgSectionTypeInvalid cfgSectionType = 0
	cfgSectionTypeNDB cfgSectionType = 1
	// cfgSectionTypeAPI cfgSectionType = 2
	cfgSectionTypeMGM cfgSectionType = 3
	// cfgSectionTypeTCP cfgSectionType = 4
	// cfgSectionTypeSHM cfgSectionType = 5
	cfgSectionTypeSystem cfgSectionType = 6
)

// readEntryFromSection reads a section from the config and extracts
// the value of the configKey entry. It allows certain filters like
// the sectionTypeFilter, nodeIdFilter, which when passed will have
// to match the section being read or else, the section is skipped.
func (cr *configReader) readEntryFromSection(
	isNodesSection bool, sectionTypeFilter cfgSectionType, nodeIdFilter uint32, configKey uint32) (stopReading bool) {

	// read the header
	sectionLength := cr.readUint32()
	numOfEntries := int(cr.readUint32())
	sectionType := cfgSectionType(cr.readUint32())

	// Calculate the offset required to skip the section.
	// Increment the current offset by sectionLength in bytes
	// minus the header size(3 words), as they have been read already.
	endOfSectionOffset := cr.offset + (sectionLength-3)*4

	if sectionType != sectionTypeFilter {
		// Caller is not interested in this type of section - skip reading configSection.
		cr.offset = endOfSectionOffset
		return false
	}

	// put the entries that need to be extracted in a map
	configValues := make(map[uint32]interface{}, 2)
	configValues[configKey] = nil
	if isNodesSection {
		// This is a node section, read the nodeId
		configValues[nodeCfgNodeId] = nil
	}

	// read the required entries and extract the values
	// stop reading the section as soon as the required values are extracted
	numOfValuesExtracted := 0
	for i := 0; i < numOfEntries && numOfValuesExtracted < len(configValues); i++ {
		key, value := cr.readEntry()
		if _, exists := configValues[key]; exists {
			// this entry needs to be stored
			configValues[key] = value
			numOfValuesExtracted++
		}
	}

	// update offset to mark end of section
	cr.offset = endOfSectionOffset

	if !isNodesSection {
		// A default section is being read
		// Store the extracted value in configReader and return
		cr.value = configValues[configKey]
		// stop reading if this is a system section, if not continue
		return sectionType == cfgSectionTypeSystem
	}

	// Node Section
	sectionNodeId := configValues[nodeCfgNodeId]
	if sectionNodeId == nil {
		debug.Panic("nodeId not found in section")
		return true
	}

	if nodeIdFilter != 0 && nodeIdFilter != sectionNodeId.(uint32) {
		// Caller not interested in this node
		return false
	}

	// store the extracted value if it is not nil
	extractedValue := configValues[configKey]
	if extractedValue != nil {
		cr.value = extractedValue
	}

	// stop reading config as both the default, and the desired node section have been read
	return true
}

// readConfig reads the config entry from the binary data and returns configValue
func (cr *configReader) readConfig(sectionTypeFilter cfgSectionType, fromNodeId, configKey uint32) configValue {

	// Start reading the header.
	//
	// Magic Word (2 words) - ignored
	// Header section (7 words) :
	// 1. Total length in words of configuration binary
	// 2. Configuration binary version (this is version 2)
	// 3. Number of default sections in configuration binary
	//    - Data node defaults
	//    - API node defaults
	//    - MGM server node defaults
	//    - TCP communication defaults
	//    - SHM communication defaults
	//    So, the value is always 5 in this version
	// 4. Number of data nodes
	// 5. Number of API nodes
	// 6. Number of MGM server nodes
	// 7. Number of communication sections

	// read the header - starts after the magic word
	cr.offset = 8
	defer func() {
		// reset offset after reading
		cr.offset = 0
	}()

	cr.totalLengthInWords = cr.readUint32()
	cr.version = cr.readUint32()
	// configReader can handle only version 2
	if cr.version != 2 {
		debug.Panic("unexpected version in get config reply")
		return nil
	}

	cr.numOfDefaultSections = cr.readUint32()
	for i := 0; i < 3; i++ {
		cr.numOfNodes += cr.readUint32()
	}
	cr.numOfCommSections = cr.readUint32()

	// Start reading the sections
	//
	// 1. The default sections
	//    - Data node defaults
	//    - API node defaults
	//    - MGM server node defaults
	//    - TCP communication defaults
	//    - SHM communication defaults
	// 2. System section
	// 3. Node sections
	//    (Ordered based on node ids of nodes of all types)
	// 4. Communication sections - ignored

	// start reading the sections
	// default sections
	for ds := 0; ds < int(cr.numOfDefaultSections); ds++ {
		// no need to handle the return value as the reading has to continue anyways
		cr.readEntryFromSection(false, sectionTypeFilter, 0, configKey)
	}

	// system section
	if cr.readEntryFromSection(false, sectionTypeFilter, 0, configKey) {
		return cr.value
	}

	// node sections
	for n := 0; n < int(cr.numOfNodes); n++ {
		if cr.readEntryFromSection(true, sectionTypeFilter, fromNodeId, configKey) {
			return cr.value
		}
	}

	// control should never reach here if the method is used right
	debug.Panic("failed to find the desired config key")
	return nil
}

// readConfigFromBase64EncodedData extracts and returns the config value
// of configKey from the given base64 encoded binary config data.
func readConfigFromBase64EncodedData(
	base64EncodedData string, sectionTypeFilter cfgSectionType, fromNodeId, configKey uint32) configValue {
	// create a config reader and read the data
	cr := getNewConfigReader(base64EncodedData)
	if cr == nil {
		return nil
	}

	return cr.readConfig(sectionTypeFilter, fromNodeId, configKey)
}
