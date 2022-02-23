// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package configparser

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	headerSection = "header"
)

// Section is a map of all config variable names and
// their value extracted from a particular section.
type Section map[string]string

// ConfigIni holds the parsed management configuration. It is a map
// of section names, and an array of all Sections with that name.
type ConfigIni map[string][]Section

// GetSection returns the Section with the sectionName.
// If multiple sections exist with the given sectionName, the method will panic.
func (ci ConfigIni) GetSection(sectionName string) Section {
	if grp, ok := ci[sectionName]; ok {
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
	return nil
}

// GetValueFromSection extracts the config value of the key from the requested
// section. If multiple sections exist with the sectionName, the method will panic.
func (ci ConfigIni) GetValueFromSection(sectionName string, key string) (value string) {
	section := ci.GetSection(sectionName)
	if section != nil {
		value = section[key]
	}
	return value
}

// GetNumberOfSections returns the number of sections with the given sectionName.
func (ci ConfigIni) GetNumberOfSections(sectionName string) int {
	if grp, ok := ci[sectionName]; ok {
		return len(grp)
	}
	return 0
}

func (ci ConfigIni) addSection(sectionName string) Section {

	grp := ci[sectionName]

	if sectionName == headerSection && len(grp) == 1 {
		// header group should have only one section, and it already exists
		return grp[0]
	}

	if grp == nil {
		// Section group doesn't exist - create one
		grp = []Section{}
		ci[sectionName] = grp
	}

	// Add the new section and return
	newSection := make(Section)
	ci[sectionName] = append(grp, newSection)
	return newSection
}

// ParseFile parses the config from the file at the
// given location into a ConfigIni object.
func ParseFile(file string) (ConfigIni, error) {

	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)

	return parseConfig(reader)
}

// ParseString parses the config string into a ConfigIni object
func ParseString(inString string) (ConfigIni, error) {

	reader := bufio.NewReader(strings.NewReader(inString))

	return parseConfig(reader)
}

// parseConfig parses the config from the reader and returns a ConfigIni object
func parseConfig(reader *bufio.Reader) (ConfigIni, error) {

	c := make(ConfigIni)

	lineno := 1
	sectionName := ""
	seenHeader := false // we only want to allow 1 header and keep track
	isComment := false

	var currentSection Section

	for {

		line, err := reader.ReadString('\n')

		// don't exit on io.EOF if there is still something read into line
		// that is e.g. the case when there is no newline at end of last line
		if err != nil && err != io.EOF {
			return c, err
		}

		if len(line) == 0 {
			break
		}
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		if line[0] == ';' || line[0] == '#' {
			line = line[1:]
			line = strings.TrimSpace(line)
			if len(line) == 0 {
				continue
			}

			// for all key value pairs in a comment we collect that under "header" section
			if !isComment {
				currentSection = c.addSection(headerSection)
				isComment = true
			}
		} else {
			// reset comment
			if isComment {
				seenHeader = true
			}
			isComment = false
		}

		if line[0] == '[' {
			if isComment {
				// no section headers in comments - only values
				continue
			}
			if line[len(line)-1] != ']' {
				return nil, fmt.Errorf("Incomplete section name in line %d %s", lineno, line)
			}

			sectionName = line[1 : len(line)-1]
			currentSection = c.addSection(sectionName)
			continue
		}

		if currentSection == nil {
			return nil, fmt.Errorf("Non-empty line without section %d %s", lineno, line)
		}

		split := strings.SplitN(line, "=", 2)
		if len(split) != 2 {
			if isComment {
				continue
			}
			return nil, fmt.Errorf("Format error %d %s", lineno, line)
		}

		if isComment && seenHeader {
			// ignore config values in comments outside header section
			continue
		}
		currentSection[split[0]] = split[1]

		lineno++
	}

	return c, nil
}
