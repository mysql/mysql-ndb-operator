package ndbtest

import (
	"github.com/onsi/ginkgo"

	deployment_utils "github.com/mysql/ndb-operator/e2e-tests/utils/deployment"
	yaml_utils "github.com/mysql/ndb-operator/e2e-tests/utils/yaml"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	ndbCRDYaml          = "deploy/charts/ndb-operator/crds/mysql.oracle.com_ndbs"
	installYamlPath     = "deploy/manifests"
	installYamlFilename = "ndb-operator"
)

// CreateNdbCRD creates the Ndb CRD in K8s cluster
func CreateNdbCRD() {
	klog.V(2).Infof("Creating Ndb Custom Resource Definition")
	RunKubectl(CreateCmd, "", yaml_utils.YamlFile("", ndbCRDYaml))
}

// DeleteNdbCRD deletes the Ndb CRD from K8s cluster
func DeleteNdbCRD() {
	klog.V(2).Infof("Deleting Ndb Custom Resource Definition")
	RunKubectl(DeleteCmd, "", yaml_utils.YamlFile("", ndbCRDYaml))
}

// required resources for ndb operator
var ndbOperatorResources = []yaml_utils.K8sObject{
	// ndb-operator service account and roles
	{
		Name:    "ndb-operator-sa",
		Kind:    "ServiceAccount",
		Version: "v1",
	},
	{
		Name:    "ndb-operator-cr",
		Kind:    "ClusterRole",
		Version: "rbac.authorization.k8s.io/v1",
	},
	{
		Name:    "ndb-operator-crb",
		Kind:    "ClusterRoleBinding",
		Version: "rbac.authorization.k8s.io/v1",
	},
	// the ndb operator itself
	{
		Name:    "ndb-operator",
		Kind:    "Deployment",
		Version: "apps/v1",
	},
}

// DeployNdbOperator deploys the Ndb operator and the required cluster roles
func DeployNdbOperator(clientset kubernetes.Interface, namespace string) {
	klog.V(2).Infof("Deploying Ndb operator")

	ginkgo.By("Creating resources for the ndb operator")
	CreateObjectsFromYaml(installYamlPath, installYamlFilename, ndbOperatorResources, namespace)

	// wait for ndb operator deployment to come up
	ginkgo.By("Waiting for ndb-operator deployment to complete")
	err := deployment_utils.WaitForDeploymentComplete(clientset, namespace, "ndb-operator")
	framework.ExpectNoError(err)
}

// UndeployNdbOperator deletes the Ndb operator and the associated cluster roles from k8s
func UndeployNdbOperator(clientset kubernetes.Interface, namespace string) {
	klog.V(2).Infof("Deleting Ndb operator")

	ginkgo.By("Deleting ndb operator resources")
	DeleteObjectsFromYaml(installYamlPath, installYamlFilename, ndbOperatorResources, namespace)

	// wait for ndb operator deployment to disappear
	ginkgo.By("Waiting for ndb-operator deployment to disappear")
	err := deployment_utils.WaitForDeploymentToDisappear(clientset, namespace, "ndb-operator")
	framework.ExpectNoError(err)
}
