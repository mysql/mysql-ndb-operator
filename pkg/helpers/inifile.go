// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package helpers

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

const headerSection = "header"

/* config variable name, config value pair */
type Section map[string]string

/*
	Multipe sections with same name will be grouped
	(such as [ndbd]) and maintained as an array in that group
*/
type ConfigIni struct {
	Groups map[string][]Section
}

func NewConfig() *ConfigIni {
	return &ConfigIni{
		Groups: make(map[string][]Section),
	}
}

func GetValueFromSingleSectionGroup(c *ConfigIni, sectionName string, key string) string {

	if grp, ok := c.Groups[sectionName]; ok {
		if len(grp) > 0 {
			if value, exists := grp[0][key]; exists {
				return value
			}
		}
	}

	return ""
}

/* ensures a new group if not exists and a new section within that group */
func (c *ConfigIni) addSection(sectionName string) *Section {

	grp := []Section{}
	if c.Groups[sectionName] == nil {
		// new group
		c.Groups[sectionName] = grp
	} else {
		grp = c.Groups[sectionName]
	}

	if sectionName == headerSection {
		// if there is a header section it should only have 1 element and we return that
		if len(grp) > 0 {
			return &grp[0]
		}
	}

	// create a new section under the new or existing group
	currentSection := &Section{}
	c.Groups[sectionName] = append(c.Groups[sectionName], *currentSection)

	return currentSection
}

func ParseFile(file string) (*ConfigIni, error) {

	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)

	return parseConfig(reader)
}

func ParseString(inString string) (*ConfigIni, error) {

	reader := bufio.NewReader(strings.NewReader(inString))

	return parseConfig(reader)
}

/*
	parses an ini configuration file and returns it
	as a config struct

	grp              e.g. [ndbd]
		section        e.g. [ndbd]
			key=value
			key=value
			...
		section        e.g. [ndbd]
	grp
		...

	all sections [section name] of same kind will be grouped under same name

	each grp is just a map of sections pointing to an array of sections:
		map[section name] = []sections

	each section has unique keys with a value:
	  map[key] = value
*/
func parseConfig(reader *bufio.Reader) (*ConfigIni, error) {

	c := NewConfig()

	lineno := 1
	sectionName := ""
	seenHeader := false // we only want to allow 1 header and keep track
	isComment := false

	var currentSection *Section = nil

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

			sectionName = string(line[1 : len(line)-1])
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
		(*currentSection)[split[0]] = split[1]

		lineno++
	}

	return c, nil
}
