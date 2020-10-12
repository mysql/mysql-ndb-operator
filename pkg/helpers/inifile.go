// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package helpers

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

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

/*
	parses an ini configuration file and returns it
	as a config struct
*/
func parseFile(file string) (*ConfigIni, error) {

	c := NewConfig()

	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	lineno := 1
	sectionName := ""
	var currentSection *Section = nil

	for {

		line := ""
		line, err = reader.ReadString('\n')
		if len(line) == 0 {
			break
		}
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		if line[0] == ';' || line[0] == '#' {
			continue
		}

		if line[0] == '[' {
			if line[len(line)-1] != ']' {
				return nil, errors.New("Incomplete section name in line " + string(lineno) + " " + line)
			}
			sectionName = string(line[1 : len(line)-1])

			if c.Groups[sectionName] == nil {
				// new group
				grp := []Section{}
				c.Groups[sectionName] = grp
			}

			currentSection = &Section{}
			c.Groups[sectionName] = append(c.Groups[sectionName], *currentSection)

			continue
		}

		if currentSection == nil {
			return nil, errors.New("Non-empty line without section" + string(lineno) + " " + line)
		}

		split := strings.Split(line, "=")
		if len(split) != 2 {
			return nil, errors.New("Format error " + string(lineno) + " " + line)
		}

		(*currentSection)[split[0]] = split[1]

		lineno++

	}

	return c, nil
}
