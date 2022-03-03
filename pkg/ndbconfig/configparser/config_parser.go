// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package configparser

import (
	"bufio"
	"fmt"
	"reflect"
	"strings"

	"github.com/mysql/ndb-operator/config/debug"
)

const (
	headerSection = "header"
)

// Section is a map of all config variable names and
// their value extracted from a particular section.
type Section map[string]string

// GetValue extracts the value of the configParam from the section
func (s Section) GetValue(configParam string) (value string, exists bool) {
	value, exists = s[strings.ToLower(configParam)]
	return value, exists
}

// SetValue sets the value of the configParam in the Section
func (s Section) SetValue(configParam string, value string) {
	s[strings.ToLower(configParam)] = value
}

// ConfigIni holds the parsed management configuration. It is a map
// of section names, and an array of all Sections with that name.
type ConfigIni map[string][]Section

// GetAllSections returns all the Sections with the sectionName.
func (ci ConfigIni) GetAllSections(sectionName string) []Section {
	return ci[strings.ToLower(sectionName)]
}

// GetSection returns the Section with the sectionName.
// If multiple sections exist with the given sectionName, the method will panic.
func (ci ConfigIni) GetSection(sectionName string) Section {
	grp := ci.GetAllSections(sectionName)
	switch len(grp) {
	case 0:
		// no Sections exist
		return nil
	case 1:
		return grp[0]
	default:
		// Wrong usage : multiple Sections exist with the same
		// name, and the method doesn't know which one to return.
		panic("GetSection : multiple Sections exist with the sectionName")
	}
}

// GetValueFromSection extracts the config value of the key from the requested
// section. If multiple sections exist with the sectionName, the method will panic.
func (ci ConfigIni) GetValueFromSection(sectionName string, key string) (value string) {
	section := ci.GetSection(sectionName)
	if section != nil {
		value, _ = section.GetValue(key)
	}
	return value
}

// GetNumberOfSections returns the number of sections with the given sectionName.
func (ci ConfigIni) GetNumberOfSections(sectionName string) int {
	return len(ci.GetAllSections(sectionName))
}

func (ci ConfigIni) addSection(sectionName string) Section {

	sectionNameInLower := strings.ToLower(sectionName)
	grp := ci[sectionNameInLower]

	if sectionName == headerSection && len(grp) == 1 {
		// header group should have only one section, and it already exists
		return grp[0]
	}

	if grp == nil {
		// Section group doesn't exist - create one
		grp = []Section{}
		ci[sectionNameInLower] = grp
	}

	// Add the new section and return
	newSection := make(Section)
	ci[sectionNameInLower] = append(grp, newSection)
	return newSection
}

// IsEqual returns if the given ConfigIni is equal to ci
func (ci ConfigIni) IsEqual(ci2 ConfigIni) bool {
	if len(ci) != len(ci2) {
		// One of the config has extra section(s) (or) one of them is nil
		return false
	}

	// Loop all sections of c1 and c2, and check if there
	// is any difference between them.
	for sectionName, sections1 := range ci {
		sections2, sectionExists := ci2[sectionName]
		if !sectionExists || len(sections1) != len(sections2) {
			// Either section with sectionName doesn't exist in c2 (or)
			// The number of sections under sectionName is not equal
			return false
		}

		// Compare the sections
		matched := make([]bool, len(sections1))
		for _, section1 := range sections1 {
			// Loop the sections in sections2 and look for a match for section1
			var matchFoundForSection1 bool
			for i, section2 := range sections2 {
				if !matched[i] && reflect.DeepEqual(section1, section2) {
					// Found a match. Mark it as matched to avoid other
					// sections getting matched against the same section again.
					matchFoundForSection1 = true
					matched[i] = true
					break
				}
			}
			if !matchFoundForSection1 {
				// No match found for Section1
				return false
			}
		}
	}

	return true
}

// ParseString parses the config string into a ConfigIni object
func ParseString(configStr string) (ConfigIni, error) {

	c := make(ConfigIni)
	var lineNo int
	var currentSection Section

	// Parse the config string using a bufio.Scanner
	scanner := bufio.NewScanner(strings.NewReader(configStr))
	for scanner.Scan() {
		// Process one line at a time
		line := strings.TrimSpace(scanner.Text())
		lineNo++
		if line == "" {
			// Empty line
			continue
		}

		// Check if this is a comment.
		// Any key value pair declared inside the first comment block
		// on top of the config, before any section is declared, will
		// be collected under the "header" section.
		var isComment bool
		if line[0] == ';' || line[0] == '#' {
			isComment = true
			line = strings.TrimSpace(strings.TrimLeft(line, ";#"))
			if line == "" {
				// No more text in comment
				continue
			}

			if len(c) == 0 {
				// No sections have been declared yet and this is the first comment block.
				// Collect any key=value pairs under the section "header"
				currentSection = c.addSection(headerSection)
			} else if len(c) == 1 && c.GetSection(headerSection) != nil {
				// First comment block and headerSection is being read
				currentSection = c.GetSection(headerSection)
			} else {
				// We can ignore this comment
				continue
			}
		} else if line[0] == '[' {
			// A section starts
			if line[len(line)-1] != ']' {
				return nil, fmt.Errorf("Incomplete section name at line %d : %s", lineNo, line)
			}

			// create/load the section
			sectionName := line[1 : len(line)-1]
			currentSection = c.addSection(sectionName)
			continue
		}

		if currentSection == nil {
			// No section is currently being read
			return nil, fmt.Errorf("Non-empty line without section at line %d : %s", lineNo, line)
		}

		// Split the line to look for a key value pair
		tokens := strings.SplitN(line, "=", 2)
		if len(tokens) != 2 || tokens[1] == "" {
			if isComment {
				// Ignore errors in a comment
				continue
			}
			return nil, fmt.Errorf("Format error at line %d : %s", lineNo, line)
		}

		// store the config key value pair
		currentSection.SetValue(tokens[0], tokens[1])
	}

	if err := scanner.Err(); err != nil {
		// Error occurred during config scan
		return nil, err
	}

	return c, nil
}

// ConfigEqual returns if the given two config strings are equal
func ConfigEqual(config1 string, config2 string) bool {
	// Parse both the configs
	c1, err := ParseString(config1)
	if err != nil {
		debug.Panic(fmt.Sprintf("ConfigEqual failed : %s", err))
		return false
	}

	c2, err := ParseString(config2)
	if err != nil {
		debug.Panic(fmt.Sprintf("ConfigEqual failed : %s", err))
		return false
	}

	return c1.IsEqual(c2)
}
