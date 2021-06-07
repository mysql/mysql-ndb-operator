package ndbtest

import (
	deployment_utils "github.com/mysql/ndb-operator/e2e-tests/utils/deployment"
	event_utils "github.com/mysql/ndb-operator/e2e-tests/utils/events"
	sfset_utils "github.com/mysql/ndb-operator/e2e-tests/utils/statefulset"
	yaml_utils "github.com/mysql/ndb-operator/e2e-tests/utils/yaml"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/controllers"

	"github.com/onsi/ginkgo"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
)

// waitForNdbSync waits for a ReasonSyncSuccess event to occur
func waitForNdbSync(clientset kubernetes.Interface, namespace string, lastKnownEventResourceVersion string) {
	err := event_utils.WaitForEvent(clientset, namespace, controllers.ReasonSyncSuccess, lastKnownEventResourceVersion)
	framework.ExpectNoError(err, "timed out waiting for operator to send sync event")
}

func waitForNdbDelete(clientset kubernetes.Interface, namespace, ndbName string) {
	err := sfset_utils.WaitForStatefulSetToDisappear(clientset, namespace, ndbName+"-ndbd")
	framework.ExpectNoError(err, "timed out waiting for ndb statefulset to disappear")
	err = sfset_utils.WaitForStatefulSetToDisappear(clientset, namespace, ndbName+"-mgmd")
	framework.ExpectNoError(err, "timed out waiting for mgmd statefulset to disappear")
	err = deployment_utils.WaitForDeploymentToDisappear(clientset, namespace, ndbName+"-mysqld")
	framework.ExpectNoError(err, "timed out waiting for mysqld deployment to disappear")
}

// KubectlApplyNdbYaml creates/updates an Ndb K8s object from the given file
// and waits for the ndb operator to setup the MySQL Cluster
func KubectlApplyNdbYaml(clientset kubernetes.Interface, namespace, path, filename string) {
	klog.V(2).Infof("creating/updating Ndb resource from %s/%s", path, filename)

	ginkgo.By("creating/updating the Ndb resource")
	lastKnownEventResourceVersion := event_utils.GetLastKnownEventResourceVersion(clientset, namespace)
	RunKubectl(ApplyCmd, namespace, yaml_utils.YamlFile(path, filename))
	waitForNdbSync(clientset, namespace, lastKnownEventResourceVersion)
}

// KubectlDeleteNdbYaml deletes an Ndb K8s object from the given file
// and waits for MySQL Cluster to shutdown
func KubectlDeleteNdbYaml(clientset kubernetes.Interface, namespace, ndbName, path, filename string) {
	klog.V(2).Infof("Deleting Ndb resource from %s/%s", path, filename)

	ginkgo.By("deleting the Ndb resource and waiting for the workloads to disappear")
	RunKubectl(DeleteCmd, namespace, yaml_utils.YamlFile(path, filename))
	waitForNdbDelete(clientset, namespace, ndbName)
}

// KubectlApplyNdbObj creates a Ndb K8s object from the given Ndb object
// and waits for the ndb operator to setup the MySQL Cluster
func KubectlApplyNdbObj(clientset kubernetes.Interface, ndb *v1alpha1.Ndb) {
	klog.V(2).Infof("creating/updating Ndb resource from %s", ndb.Namespace)

	yamlContent := yaml_utils.MarshalNdb(ndb)

	ginkgo.By("creating/updating the Ndb resource")
	lastKnownEventResourceVersion := event_utils.GetLastKnownEventResourceVersion(clientset, ndb.Namespace)
	RunKubectl(ApplyCmd, ndb.Namespace, string(yamlContent))
	waitForNdbSync(clientset, ndb.Namespace, lastKnownEventResourceVersion)
}

// KubectlDeleteNdbObj deletes an Ndb K8s object from the given Ndb object
// and waits for MySQL Cluster to shutdown
func KubectlDeleteNdbObj(clientset kubernetes.Interface, ndb *v1alpha1.Ndb) {
	klog.V(2).Infof("Deleting Ndb resource from %s", ndb.Namespace)

	yamlContent := yaml_utils.MarshalNdb(ndb)

	ginkgo.By("deleting the Ndb resource and waiting for the workloads to disappear")
	RunKubectl(DeleteCmd, ndb.Namespace, string(yamlContent))
	waitForNdbDelete(clientset, ndb.Namespace, ndb.Name)
}
