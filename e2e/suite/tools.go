package e2e

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v2"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/testfiles"
)

func YamlFile(test, file string) string {
	from := filepath.Join(test, file+".yaml")
	data, err := testfiles.Read(from)
	if err != nil {
		dir, _ := os.Getwd()
		klog.Infof("Maybe in wrong directory %s", dir)
		framework.Fail(err.Error())
	}
	return string(data)
}

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

func createOrDeleteFromYaml(ns string, path string, filename string, command string) {

	if command != "delete" && command != "create" {
		return
	}

	y := YamlFile(path, filename)

	if ns != "" {
		var err error
		y, err = ReplaceAllProperties(y, "namespace", ns)
		if err != nil {
			klog.Fatalf("Error parsing %s\n", y)
		}
	}

	framework.RunKubectlOrDieInput(ns, y, command, "-n", ns, "-f", "-")
}

// CreateFromYaml creates a resource from a yaml file
// in a specfic namespace and with kubectl
func CreateFromYaml(ns string, path string, filename string) {
	createOrDeleteFromYaml(ns, path, filename, "create")
}

// CreateFromYamls creates a resources from resource paths given in an array
func CreateFromYamls(ns string, resourceFiles []string) {
	for _, d := range resourceFiles {
		CreateFromYaml(ns, "", d)
	}
}

// DeleteFromYaml deletes resources given in a yaml file
func DeleteFromYaml(ns string, path string, filename string) {
	createOrDeleteFromYaml(ns, path, filename, "delete")
}

// DeleteFromYamls deletes resources from resource paths given in an array
func DeleteFromYamls(ns string, resourceFiles []string) {
	for _, d := range resourceFiles {
		DeleteFromYaml(ns, "", d)
	}
}
