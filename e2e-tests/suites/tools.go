package e2e

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/testfiles"

	deploymentutil "k8s.io/kubernetes/pkg/controller/deployment/util"
	e2edeployment "k8s.io/kubernetes/test/e2e/framework/deployment"
)

// YamlFile reads the content of a single yamle file as string
// Read happens from a test files path which needs to first
// be registered with testfiles.AddFileSource()
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

// CreateDeploymentFromSpec creates a deployment.
// copied from k8s.io/kubernetes@v1.18.2/test/e2e/framework/deployment/fixtures.go
// but using own deployment instead
func CreateDeploymentFromSpec(client clientset.Interface, deployment *appsv1.Deployment) (*appsv1.Deployment, error) {
	deployment, err := client.AppsV1().Deployments(deployment.Namespace).Create(context.TODO(), deployment, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("deployment %q Create API error: %v", deployment.Name, err)
	}
	framework.Logf("Waiting deployment %q to complete", deployment.Name)
	err = e2edeployment.WaitForDeploymentComplete(client, deployment)
	if err != nil {
		return nil, fmt.Errorf("deployment %q failed to complete: %v", deployment.Name, err)
	}
	return deployment, nil
}

// WaitForDeploymentComplete waits for the deployment to complete.
// copied and modified from k8s.io/kubernetes@v1.18.2/test/e2e/framework/
func WaitForDeploymentComplete(c clientset.Interface, namespace, name string, pollInterval, pollTimeout time.Duration) error {
	var (
		deployment *appsv1.Deployment
		reason     string
	)

	err := wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		var err error
		deployment, err = c.AppsV1().Deployments(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		// When the deployment status and its underlying resources reach the desired state, we're done
		if deploymentutil.DeploymentComplete(deployment, &deployment.Status) {
			return true, nil
		}

		reason = fmt.Sprintf("deployment status: %#v", deployment.Status)
		klog.Info(reason)

		return false, nil
	})

	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("%s", reason)
	}
	if err != nil {
		return fmt.Errorf("error waiting for deployment %q status to match expectation: %v", name, err)
	}
	return nil
}

// statefulSetComplete tests if a statefulset is up and running
func statefulSetComplete(sfset *appsv1.StatefulSet, newStatus *appsv1.StatefulSetStatus) bool {
	return newStatus.UpdatedReplicas == *(sfset.Spec.Replicas) &&
		newStatus.ReadyReplicas == *(sfset.Spec.Replicas) &&
		newStatus.Replicas == *(sfset.Spec.Replicas) &&
		newStatus.ObservedGeneration >= sfset.Generation
}

// WaitForStatefulSetComplete waits for a statefulset to complete.
// adopted from WaitForDeploymentComplete
func WaitForStatefulSetComplete(c clientset.Interface, namespace, name string, pollInterval, pollTimeout time.Duration) error {
	var (
		sfset  *appsv1.StatefulSet
		reason string
	)

	err := wait.PollImmediate(pollInterval, pollTimeout, func() (bool, error) {
		var err error
		sfset, err = c.AppsV1().StatefulSets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		// When the deployment status and its underlying resources reach the desired state, we're done
		if statefulSetComplete(sfset, &sfset.Status) {
			return true, nil
		}

		reason = fmt.Sprintf("statefulset status: %#v", sfset.Status)
		klog.Info(reason)

		return false, nil
	})

	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("%s", reason)
	}
	if err != nil {
		return fmt.Errorf("error waiting for deployment %q status to match expectation: %v", name, err)
	}
	return nil
}
