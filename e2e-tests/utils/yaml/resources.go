package yaml

import (
	"k8s.io/klog"

	"k8s.io/kubernetes/test/e2e/framework"
)

func createOrDeleteFromYaml(ns string, path string, filename string, command string) {

	if command != "delete" && command != "create" {
		klog.Fatalf("Wrong command type given %s\n", command)
		return
	}

	y := YamlFile(path, filename)

	if ns != "" {
		var err error
		y, err = ReplaceAllProperties(y, "namespace", ns)
		if err != nil {
			klog.Fatalf("Error parsing %s\n", y)
			return
		}
	}

	result := framework.RunKubectlOrDieInput(ns, y, command, "-n", ns, "-f", "-")
	klog.Infof("kubectl executing %s on %s: %s", command, filename, result)
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
