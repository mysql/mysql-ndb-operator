// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package yaml

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
)

func replaceAll(values *interface{}, key string, newValue string) {

	switch (*values).(type) {
	case []interface{}:
		for _, v := range (*values).([]interface{}) {
			replaceAll(&v, key, newValue)
		}
		break

	case map[interface{}]interface{}:
		valuesMap := (*values).(map[interface{}]interface{})
		for k, v := range valuesMap {
			if key == k {
				(valuesMap)[key] = newValue
			} else {
				replaceAll(&v, key, newValue)
			}
		}
		break

	default:
		// likely "just" the value, iteration endpoint
		break
	}
}

// ReplaceAllProperties replaces all occurences of a property with a new string key/value pair
// e.g. "<content>", "namespace", "new-namespace" will replace all
// occurences of the property "namespace" with "new-namespace".
// Search happens in all arrays and sub-objects as well.
func ReplaceAllProperties(content string, key string, newValue string) (string, error) {

	// remove all helm artifacts - i.e. {{ }}
	m1 := regexp.MustCompile(`\{\{(.*?)\}\}`)
	contentS := m1.ReplaceAllString(string(content), "")

	// use Decoder API in order to cope with multiple yamls in one file
	reader := strings.NewReader(string(contentS))
	dec := yaml.NewDecoder(reader)

	var value interface{}

	result := ""
	for {
		err := dec.Decode(&value)
		if err == io.EOF {
			break
		}

		if err != nil {
			fmt.Println(err)
			return "", err
		}

		replaceAll(&value, key, newValue)

		out, _ := yaml.Marshal(value)

		if len(result) > 0 {
			result += fmt.Sprintf("\n---\n")
		}
		result += string(out)
	}

	return result, nil
}
