package config

import (
	"encoding/base64"
	"encoding/binary"
)

type Configuration struct {
	data     []byte
	cursor   int
	sections []*Section
	system   int
}

func NewConfigurationFromBase64(base64Config string) (*Configuration, error) {
	var err error
	var nSections int
	var c *Configuration = new(Configuration)

	c.data, err = base64.StdEncoding.DecodeString(base64Config)
	if err == nil {
		header := c.data[8:36]
		nSections = int(binary.BigEndian.Uint32(header[8:12])) // defaults
		c.system = nSections
		nSections += 1                                           // SYSTEM
		nSections += int(binary.BigEndian.Uint32(header[12:16])) // DB
		nSections += int(binary.BigEndian.Uint32(header[16:20])) // API
		nSections += int(binary.BigEndian.Uint32(header[20:24])) // MGM
		nSections += int(binary.BigEndian.Uint32(header[24:28])) // TCP
		c.cursor = 36
	}
	c.sections = make([]*Section, nSections)
	for i := 0; i < nSections; i++ {
		c.sections[i] = c.readSection()
	}

	return c, err
}

func (c *Configuration) SystemSection() *ConfigSection {
	return &c.sections[c.system].values
}

type ConfigValueType interface{}
type ConfigSection map[int32]ConfigValueType
type Section struct {
	length      int
	sectionType ConfigSectionType
	values      ConfigSection
	next        *Section
}

func (cs *ConfigSection) GenerationNumber() int {
	if val, ok := (*cs)[int32(CFG_SYS_CONFIG_GENERATION)]; ok {
		if v, ok := val.(uint32); ok {
			return int(v)
		}
		if v, ok := val.(uint64); ok {
			return int(v)
		}
	}
	return -1
}

// From include/util/ConfigSection.hpp

type KeyType int
type SectionType int
type ConfigSectionType int

const (
	InvalidTypeId = 0 // KeyType
	IntTypeId     = 1
	StringTypeId  = 2
	SectionTypeId = 3
	Int64TypeId   = 4
)

const (
	InvalidSectionTypeId = 0 // SectionType
	DataNodeTypeId       = 1
	ApiNodeTypeId        = 2
	MgmNodeTypeId        = 3
	TcpTypeId            = 4
	ShmTypeId            = 5
	SystemSectionId      = 6
)

const (
	InvalidConfigSection = 0 // ConfigSectionType
	BaseSection          = 1
	NodePointerSection   = 2
	CommPointerSection   = 3
	SystemPointerSection = 4
	NodeSection          = 5
	CommSection          = 6
	SystemSection        = 7
)

func getKeyType(k uint32) KeyType {
	const V2_TYPE_SHIFT = 28
	const V2_TYPE_MASK = 15
	return KeyType((k >> V2_TYPE_SHIFT) & V2_TYPE_MASK)
}

func getKey(k uint32) uint32 {
	const V2_KEY_MASK = 0x0FFFFFFF
	return k & V2_KEY_MASK
}

func readUint32(data []byte, offset *int) uint32 {
	v := binary.BigEndian.Uint32(data[*offset : *offset+4])
	*offset += 4
	return v
}

func readString(data []byte, offset *int) string {
	len := int(readUint32(data, offset))
	s := string(data[*offset : *offset+len])
	len = len + ((4 - (len & 3)) & 3)
	*offset += len
	return s
}

func (c *Configuration) readEntry() (uint32, interface{}) {
	k := readUint32(c.data, &c.cursor)
	key_type := getKeyType(k)
	key := getKey(k)

	switch key_type {
	case IntTypeId:
		val := readUint32(c.data, &c.cursor)
		return key, val
	case Int64TypeId:
		high := readUint32(c.data, &c.cursor)
		low := readUint32(c.data, &c.cursor)
		val := (uint64(high) << 32) + uint64(low)
		return key, val
	case StringTypeId:
		val := readString(c.data, &c.cursor)
		return key, val
	default:
		return key, nil
	}
}

func (c *Configuration) readSection() *Section {
	var section *Section = new(Section)
	var noEntries int

	section.length = int(readUint32(c.data, &c.cursor))
	noEntries = int(readUint32(c.data, &c.cursor))
	section.sectionType = ConfigSectionType(readUint32(c.data, &c.cursor))

	section.values = make(ConfigSection, noEntries)
	for e := 0; e < noEntries; e++ {
		key, val := c.readEntry()
		section.values[int32(key)] = val
	}
	return section
}
