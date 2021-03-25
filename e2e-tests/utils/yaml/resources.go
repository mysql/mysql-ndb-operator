package yaml

import (
	"fmt"
	"github.com/onsi/gomega"
	"gopkg.in/yaml.v2"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
	"strings"
)

func createOrDeleteFromYaml(ns string, path string, filename string, command string) string {

	if command != "delete" && command != "create" {
		klog.Errorf("Wrong command type given %s\n", command)
		return ""
	}

	y := YamlFile(path, filename)

	if ns != "" {
		var err error
		y, err = ReplaceAllProperties(y, "namespace", ns)
		if err != nil {
			klog.Errorf("Error parsing %s\n", y)
			return ""
		}
	}

	result := framework.RunKubectlOrDieInput(ns, y, command, "-n", ns, "-f", "-")
	klog.V(3).Infof("kubectl executing %s on %s: %s", command, filename, result)
	return result
}

// CreateFromYaml creates a resource from a yaml file
// in a specfic namespace and with kubectl
func CreateFromYaml(ns string, path string, filename string) {
	res := createOrDeleteFromYaml(ns, path, filename, "create")
	gomega.Expect(strings.Count(res, "created")).To(gomega.Equal(1))
}

// CreateFromYamls creates a resources from resource paths given in an array
func CreateFromYamls(ns string, resourceFiles []string) {
	for _, d := range resourceFiles {
		CreateFromYaml(ns, "", d)
	}
}

// DeleteFromYaml deletes resources given in a yaml file
func DeleteFromYaml(ns string, path string, filename string) {
	res := createOrDeleteFromYaml(ns, path, filename, "delete")
	gomega.Expect(strings.Count(res, "deleted")).To(gomega.Equal(1))
}

// DeleteFromYamls deletes resources from resource paths given in an array
func DeleteFromYamls(ns string, resourceFiles []string) {
	for _, d := range resourceFiles {
		DeleteFromYaml(ns, "", d)
	}
}

// K8sObject identifies a K8s object in a yaml file
type K8sObject struct {
	Name, Kind, Version string
}

// extractObjectsFromYaml extracts the resource identified by
// the Kind, Version, Name from the given yaml file
func extractObjectsFromYaml(path, filename string, k8sObjects []K8sObject, ns string) string {

	// generate a map of k8s object identifiers with
	// key name;kind;version for easier lookup
	k8sObjMap := make(map[string]struct{})
	for _, obj := range k8sObjects {
		k8sObjMap[obj.Name+";"+obj.Kind+";"+obj.Version] = struct{}{}
	}

	// read the given file
	yamlFile := YamlFile(path, filename)

	// split the various docs in the file, read one by one
	// and extract them into a single list
	var objsToBeCreated []string
	yamlDocs := strings.Split(yamlFile, "---")
	for _, yamlDoc := range yamlDocs {
		if len(k8sObjMap) == 0 {
			// Found all objects to be created
			break
		}

		var resource interface{}
		err := yaml.Unmarshal([]byte(yamlDoc), &resource)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		if resource == nil {
			// empty yaml doc
			continue
		}

		// non empty yaml doc - extract the name/kind/version
		resourceMap := resource.(map[interface{}]interface{})
		metadata := resourceMap["metadata"].(map[interface{}]interface{})
		name := metadata["name"].(string)
		kind := resourceMap["kind"].(string)
		apiVersion := resourceMap["apiVersion"].(string)
		// if this object exists in the map, update and append it to output
		objKey := name + ";" + kind + ";" + apiVersion
		if _, exists := k8sObjMap[objKey]; exists {
			// replace namespace if required
			if ns != "" {
				replaceAll(&resource, "namespace", ns)
			}

			// append the resource to the list
			updatedDoc, err := yaml.Marshal(resource)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			objsToBeCreated = append(objsToBeCreated, string(updatedDoc))

			// delete the key from map
			delete(k8sObjMap, objKey)
		}
	}

	if len(k8sObjMap) != 0 {
		// few resources not found
		var errorMsg []string
		for key, _ := range k8sObjMap {
			res := strings.Split(key, ";")
			errorMsg = append(errorMsg, fmt.Sprintf(
				"Requested resource '%s' with kind '%s' and version '%s' not found", res[0], res[1], res[2]))
		}
		framework.Fail(strings.Join(errorMsg, "\n"))
	}

	// Return the docs as a single yaml string
	return strings.Join(objsToBeCreated, "\n---\n")
}

// CreateObjectsFromYaml extracts the resource identified by the
// Kind, Version, Name and creates that in the k8s cluster
// If the ns string is not empty, the resource will be created
// in the namespace pointed by it
func CreateObjectsFromYaml(path, filename string, k8sObjects []K8sObject, ns string) {
	// Apply the yaml to k8s
	kubectlInput := extractObjectsFromYaml(path, filename, k8sObjects, ns)
	gomega.Expect(kubectlInput).NotTo(gomega.BeEmpty())
	result := framework.RunKubectlOrDieInput(ns, kubectlInput, "apply", "-n", ns, "-f", "-")
	klog.V(3).Infof("kubectl executing %s on some objects from %s: %s", "apply", filename, result)
	gomega.Expect(strings.Count(result, "created")).To(gomega.Equal(len(k8sObjects)))
}

// DeleteObjectsFromYaml extracts the resource identified by the
// Kind, Version, Name and deletes them from the k8s cluster
// If the ns string is not empty, the resource will be deleted
// from the namespace pointed by it
func DeleteObjectsFromYaml(path, filename string, k8sObjects []K8sObject, ns string) {
	// Apply the yaml to k8s
	kubectlInput := extractObjectsFromYaml(path, filename, k8sObjects, ns)
	gomega.Expect(kubectlInput).NotTo(gomega.BeEmpty())
	result := framework.RunKubectlOrDieInput(ns, kubectlInput, "delete", "-n", ns, "-f", "-")
	klog.V(3).Infof("kubectl executing %s on some objects from %s: %s", "delete", filename, result)
	gomega.Expect(strings.Count(result, "deleted")).To(gomega.Equal(len(k8sObjects)))
}
