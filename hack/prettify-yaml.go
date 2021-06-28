// +build ignore

// Tool to prettify a yaml file

package main

import (
	"bytes"
	"flag"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
)

var (
	filePath string
)

func init() {
	flag.StringVar(&filePath, "yaml", "",
		"Path of the yaml file that needs to be prettified")
}

func main() {
	flag.Parse()
	log.SetFlags(log.Lshortfile)

	if filePath == "" {
		flag.Usage()
		os.Exit(1)
	}

	// read the file
	yamlFile, err := os.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read file '%s' : %s", filePath, err)
	}

	yamlDocs := bytes.Split(yamlFile, []byte("---"))

	// prettify the yaml docs one by one
	var prettifiedYamlFile string
	for _, doc := range yamlDocs {
		// Unmarshal, Marshal and print
		m := make(map[interface{}]interface{})

		err = yaml.Unmarshal(doc, &m)
		if err != nil {
			log.Fatalf("Failed to unmarshal doc : %s", err)
		}

		if len(m) > 0 {
			prettyDoc, err := yaml.Marshal(&m)
			if err != nil {
				log.Fatalf("Failed to marshal doc : %s", err)
			}
			prettifiedYamlFile += "---\n"
			prettifiedYamlFile += string(prettyDoc)
		}
	}

	// write back into the yaml file
	if err = ioutil.WriteFile(filePath, []byte(prettifiedYamlFile), os.ModeExclusive); err != nil {
		log.Fatalf("Failed to write file '%s' : %s", filePath, err)
	}
}
