// Copyright (c) 2021, 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
)

type configValue interface{}

// configReader helps in extracting config information from a given binary config data
type configReader struct {
	// raw data and offset
	data   []byte
	offset int

	// extracted information
	totalLengthInWords, version                         uint32
	numOfDefaultSections, numOfNodes, numOfCommSections int
	value                                               configValue
}

const (
	configMagic            = "NDBCONF2"
	configHeaderLength     = 9 * 4
	configSectionHeadWords = 3
)

// getNewConfigReader creates a new configReader
// for the given base 64 encoded config data
func getNewConfigReader(base64EncodedData string) (*configReader, error) {
	// decode the string
	decodedData, err := base64.StdEncoding.DecodeString(base64EncodedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode config string: %w", err)
	}

	return &configReader{data: decodedData}, nil
}

// readUint32 reads an uint32 from the data at the current offset
func (cr *configReader) readUint32(limit int) (uint32, error) {
	if limit < 0 || limit > len(cr.data) {
		return 0, fmt.Errorf("Invalid limit while reading config: %d", limit)
	}
	// Check subtraction before slicing so a malformed length cannot overflow.
	if cr.offset < 0 || cr.offset+4 > limit {
		return 0, fmt.Errorf("unexpected end of config at byte %d", cr.offset)
	}
	v := binary.BigEndian.Uint32(cr.data[cr.offset : cr.offset+4])
	cr.offset += 4
	return v, nil
}

// readString reads a string from the data at the current offset
func (cr *configReader) readString(limit int) (string, error) {
	// Extract the length of the string
	strLen, err := cr.readUint32(limit)
	if err != nil {
		return "", err
	}
	if strLen == 0 {
		return "", fmt.Errorf("invalid zero-length string at byte %d", cr.offset-4)
	}

	// Include alignment padding to keep the input aligned on a four-byte boundary,
	// while keeping the calculation overflow-safe.
	paddedLength := (uint64(strLen) + 3) &^ uint64(3)
	if paddedLength > uint64(limit-cr.offset) {
		return "", fmt.Errorf("string at byte %d exceeds its section", cr.offset-4)
	}
	stringEnd := cr.offset + int(strLen)
	if cr.data[stringEnd-1] != 0 {
		return "", fmt.Errorf("string at byte %d is not null-terminated", cr.offset-4)
	}

	// Extract the string.
	// Null character is present at the end and should not be read.
	s := string(cr.data[cr.offset : stringEnd-1])
	// update offset
	cr.offset += int(paddedLength)
	return s, nil
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
func (cr *configReader) readEntry(limit int) (uint32, configValue, error) {

	// Extract key and key type
	keyAndType, err := cr.readUint32(limit)
	if err != nil {
		return 0, nil, err
	}
	key := (keyAndType >> v2KeyShift) & v2KeyMask
	keyType := (keyAndType >> v2TypeShift) & v2TypeMask

	// Read the value based on the key type
	var value interface{}
	switch keyType {
	case CfgEntryTypeInt:
		{
			value, err = cr.readUint32(limit)
		}
	case CfgEntryTypeInt64:
		{
			var high, low uint32
			high, err = cr.readUint32(limit)
			if err == nil {
				low, err = cr.readUint32(limit)
			}
			value = (uint64(high) << 32) + uint64(low)
		}
	case CfgEntryTypeString:
		{
			value, err = cr.readString(limit)
		}
	default:
		return 0, nil, fmt.Errorf("unsupported config entry type %d at byte %d", keyType, cr.offset-4)
	}
	if err != nil {
		return 0, nil, err
	}

	return key, value, nil
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
	isNodesSection bool, sectionTypeFilter cfgSectionType, nodeIdFilter uint32, configKey uint32) (
	stopReading bool, err error) {

	// read the header
	sectionStart := cr.offset
	limit := len(cr.data)
	sectionLength, err := cr.readUint32(limit)
	if err != nil {
		return false, err
	}
	if sectionLength < configSectionHeadWords {
		return false, fmt.Errorf("invalid section length %d at byte %d", sectionLength, sectionStart)
	}
	numOfEntries, err := cr.readUint32(limit)
	if err != nil {
		return false, err
	}
	sectionTypeValue, err := cr.readUint32(limit)
	if err != nil {
		return false, err
	}
	sectionType := cfgSectionType(sectionTypeValue)

	// Calculate the offset required to skip the section and validate it
	payloadBytes := uint64(sectionLength-configSectionHeadWords) * 4
	remainingBytes := limit - cr.offset
	if payloadBytes > uint64(remainingBytes) {
		return false, fmt.Errorf("section at byte %d exceeds the config data", sectionStart)
	}
	// Increment the current offset by sectionLength in bytes
	// minus the header size(3 words), as they have been read already.
	endOfSectionOffset := cr.offset + int(payloadBytes)

	if sectionType != sectionTypeFilter {
		// Caller is not interested in this type of section - skip reading configSection.
		cr.offset = endOfSectionOffset
		return false, nil
	}

	// read the required entries and extract the values
	// stop reading the section as soon as the required values are extracted
	var extractedValue configValue
	var sectionNodeId uint32
	configValueFound := false
	nodeIdFound := !isNodesSection
	for entry := uint32(0); entry < numOfEntries && (!configValueFound || !nodeIdFound); entry++ {
		key, value, err := cr.readEntry(endOfSectionOffset)
		if err != nil {
			return false, fmt.Errorf("invalid entry in section at byte %d: %w", sectionStart, err)
		}
		if key == configKey {
			extractedValue = value
			configValueFound = true
		}
		if isNodesSection && key == nodeCfgNodeId {
			var validType bool
			sectionNodeId, validType = value.(uint32)
			if !validType {
				return false, fmt.Errorf("node id in section at byte %d has type %T", sectionStart, value)
			}
			nodeIdFound = true
		}
	}

	// update offset to mark end of section
	cr.offset = endOfSectionOffset

	if !isNodesSection {
		// A default section is being read
		// Store the extracted value in configReader and return
		if configValueFound {
			cr.value = extractedValue
		}
		// stop reading if this is a system section, if not continue
		return sectionType == cfgSectionTypeSystem, nil
	}

	// Node Section
	if !nodeIdFound {
		return false, fmt.Errorf("node id not found in section at byte %d", sectionStart)
	}

	if nodeIdFilter != 0 && nodeIdFilter != sectionNodeId {
		// Caller not interested in this node
		return false, nil
	}

	// store the extracted value if it is not nil
	if extractedValue != nil {
		cr.value = extractedValue
	}

	// stop reading config as both the default, and the desired node section have been read
	return true, nil
}

// readConfig reads the config entry from the binary data and returns configValue
func (cr *configReader) readConfig(
	sectionTypeFilter cfgSectionType, fromNodeId, configKey uint32) (configValue, error) {
	// Reset extracted state because tests and callers may reuse a reader.
	cr.offset = 0
	cr.totalLengthInWords = 0
	cr.version = 0
	cr.numOfDefaultSections = 0
	cr.numOfNodes = 0
	cr.numOfCommSections = 0
	cr.value = nil

	limit := len(cr.data)
	if limit < configHeaderLength {
		return nil, fmt.Errorf("config is shorter than the %d-byte header", configHeaderLength)
	}
	if limit%4 != 0 {
		return nil, fmt.Errorf("config length %d is not word-aligned", limit)
	}
	if string(cr.data[:len(configMagic)]) != configMagic {
		return nil, fmt.Errorf("unexpected config magic")
	}

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

	var err error
	cr.totalLengthInWords, err = cr.readUint32(limit)
	if err != nil {
		return nil, err
	}
	if uint64(cr.totalLengthInWords)*4 != uint64(limit) {
		return nil, fmt.Errorf(
			"declared config length %d words does not match %d bytes",
			cr.totalLengthInWords, limit)
	}

	cr.version, err = cr.readUint32(limit)
	if err != nil {
		return nil, err
	}
	// configReader can handle only version 2
	if cr.version != 2 {
		return nil, fmt.Errorf("unsupported config version %d", cr.version)
	}

	numOfDefaultSections, err := cr.readUint32(limit)
	if err != nil {
		return nil, err
	}

	numOfNodes := uint64(0)
	for i := 0; i < 3; i++ {
		nodeCount, err := cr.readUint32(limit)
		if err != nil {
			return nil, err
		}
		numOfNodes += uint64(nodeCount)
	}
	numOfCommSections, err := cr.readUint32(limit)
	if err != nil {
		return nil, err
	}

	// Every declared section needs at least its three-word header.
	minimumSectionCount := uint64(numOfDefaultSections) + 1 + numOfNodes + uint64(numOfCommSections)
	maximumSectionCount := uint64(limit-cr.offset) / (configSectionHeadWords * 4)
	if minimumSectionCount > maximumSectionCount {
		return nil, fmt.Errorf("declared section counts exceed the config length")
	}
	cr.numOfDefaultSections = int(numOfDefaultSections)
	cr.numOfNodes = int(numOfNodes)
	cr.numOfCommSections = int(numOfCommSections)

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
	for ds := 0; ds < cr.numOfDefaultSections; ds++ {
		// no need to handle the return value as the reading has to continue anyways
		if _, err := cr.readEntryFromSection(false, sectionTypeFilter, 0, configKey); err != nil {
			return nil, err
		}
	}

	// system section
	stopReading, err := cr.readEntryFromSection(false, sectionTypeFilter, 0, configKey)
	if err != nil {
		return nil, err
	}
	if stopReading {
		if cr.value == nil {
			return nil, fmt.Errorf("config key %d was not found", configKey)
		}
		return cr.value, nil
	}

	// node sections
	for n := 0; n < cr.numOfNodes; n++ {
		stopReading, err = cr.readEntryFromSection(true, sectionTypeFilter, fromNodeId, configKey)
		if err != nil {
			return nil, err
		}
		if stopReading {
			if cr.value == nil {
				return nil, fmt.Errorf("config key %d was not found", configKey)
			}
			return cr.value, nil
		}
	}

	return nil, fmt.Errorf("config key %d was not found", configKey)
}

// readConfigFromBase64EncodedData extracts and returns the config value
// of configKey from the given base64 encoded binary config data.
func readConfigFromBase64EncodedData(
	base64EncodedData string, sectionTypeFilter cfgSectionType, fromNodeId, configKey uint32) (configValue, error) {
	// create a config reader and read the data
	cr, err := getNewConfigReader(base64EncodedData)
	if err != nil {
		return nil, err
	}

	return cr.readConfig(sectionTypeFilter, fromNodeId, configKey)
}
