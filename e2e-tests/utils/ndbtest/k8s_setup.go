package ndbtest

import (
	deployment_utils "github.com/mysql/ndb-operator/e2e-tests/utils/deployment"
	event_utils "github.com/mysql/ndb-operator/e2e-tests/utils/events"
	sfset_utils "github.com/mysql/ndb-operator/e2e-tests/utils/statefulset"
	yaml_utils "github.com/mysql/ndb-operator/e2e-tests/utils/yaml"
	"github.com/onsi/ginkgo"

	"github.com/mysql/ndb-operator/pkg/controllers"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	ndbCRDYaml          = "helm/crds/mysql.oracle.com_ndbs"
	installYamlPath     = "artifacts/install"
	installYamlFilename = "ndb-operator"
)

// CreateNdbCRD creates the Ndb CRD in K8s cluster
func CreateNdbCRD() {
	klog.V(2).Infof("Creating Ndb Custom Resource Definition")
	yaml_utils.CreateFromYaml("", "", ndbCRDYaml)
}

// DeleteNdbCRD deletes the Ndb CRD from K8s cluster
func DeleteNdbCRD() {
	klog.V(2).Infof("Deleting Ndb Custom Resource Definition")
	yaml_utils.DeleteFromYaml("", "", ndbCRDYaml)
}

// required resources for ndb operator
var ndbOperatorResources = []yaml_utils.K8sObject{
	// ndb-agent service account and roles
	{"ndb-agent", "ServiceAccount", "v1"},
	{"ndb-agent", "ClusterRole", "rbac.authorization.k8s.io/v1"},
	{"ndb-agent", "ClusterRoleBinding", "rbac.authorization.k8s.io/v1"},
	// ndb-operator service account and roles
	{"ndb-operator", "ServiceAccount", "v1"},
	{"ndb-operator", "ClusterRole", "rbac.authorization.k8s.io/v1"},
	{"ndb-operator", "ClusterRoleBinding", "rbac.authorization.k8s.io/v1"},
	// the ndb operator itself
	{"ndb-operator", "Deployment", "apps/v1"},
}

// DeployNdbOperator deploys the Ndb operator and the required cluster roles
func DeployNdbOperator(clientset kubernetes.Interface, namespace string) {
	klog.V(2).Infof("Deploying Ndb operator")

	ginkgo.By("Creating resources for the ndb operator")
	yaml_utils.CreateObjectsFromYaml(installYamlPath, installYamlFilename, ndbOperatorResources, namespace)

	// wait for ndb operator deployment to come up
	ginkgo.By("Waiting for ndb-operator deployment to complete")
	err := deployment_utils.WaitForDeploymentComplete(clientset, namespace, "ndb-operator")
	framework.ExpectNoError(err)
}

// UndeployNdbOperator deletes the Ndb operator and the associated cluster roles from k8s
func UndeployNdbOperator(clientset kubernetes.Interface, namespace string) {
	klog.V(2).Infof("Deleting Ndb operator")

	ginkgo.By("Deleting ndb operator resources")
	yaml_utils.DeleteObjectsFromYaml(installYamlPath, installYamlFilename, ndbOperatorResources, namespace)

	// wait for ndb operator deployment to disappear
	ginkgo.By("Waiting for ndb-operator deployment to disappear")
	err := deployment_utils.WaitForDeploymentToDisappear(clientset, namespace, "ndb-operator")
	framework.ExpectNoError(err)
}

// CreateNdbResource creates a Ndb object from the given file
// and waits for the ndb operator to setup the MySQL Cluster
func CreateNdbResource(clientset kubernetes.Interface, namespace, path, filename string) {
	klog.V(2).Infof("Creating Ndb resource from %s/%s", path, filename)

	ginkgo.By("creating the Ndb resource")
	lastKnownEventResourceVersion := event_utils.GetLastKnownEventResourceVersion(clientset, namespace)
	yaml_utils.CreateFromYaml(namespace, path, filename)

	err := event_utils.WaitForEvent(clientset, namespace, controllers.ReasonSyncSuccess, lastKnownEventResourceVersion)
	framework.ExpectNoError(err, "timed out waiting for operator to send sync event")
}

// DeleteNdbResource deletes the Ndb object from the given file
// and waits for MySQL Cluster to shutdown
func DeleteNdbResource(clientset kubernetes.Interface, namespace, ndbName, path, filename string) {
	klog.V(2).Infof("Deleting Ndb resource from %s/%s", path, filename)

	ginkgo.By("deleting the Ndb resource and waiting for the workloads to disappear")
	yaml_utils.DeleteFromYaml(namespace, path, filename)
	err := sfset_utils.WaitForStatefulSetToDisappear(clientset, namespace, ndbName+"-ndbd")
	framework.ExpectNoError(err, "timed out waiting for ndb statefulset to disappear")
	err = sfset_utils.WaitForStatefulSetToDisappear(clientset, namespace, ndbName+"-mgmd")
	framework.ExpectNoError(err, "timed out waiting for mgmd statefulset to disappear")
	err = deployment_utils.WaitForDeploymentToDisappear(clientset, namespace, ndbName+"-mysqld")
	framework.ExpectNoError(err, "timed out waiting for mysqld deployment to disappear")
}
