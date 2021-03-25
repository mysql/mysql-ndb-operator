package ndbtest

import (
	"fmt"
	deployment_utils "github.com/mysql/ndb-operator/e2e-tests/utils/deployment"
	yaml_utils "github.com/mysql/ndb-operator/e2e-tests/utils/yaml"
	"github.com/onsi/ginkgo"

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
	ginkgo.By(fmt.Sprintf("Waiting for ndb-operator deployment to complete"))
	err := deployment_utils.WaitForDeploymentComplete(clientset, namespace, "ndb-operator")
	framework.ExpectNoError(err)
}

// UndeployNdbOperator deletes the Ndb operator and the associated cluster roles from k8s
func UndeployNdbOperator(clientset kubernetes.Interface, namespace string) {
	klog.V(2).Infof("Deleting Ndb operator")

	ginkgo.By("Deleting ndb operator resources")
	yaml_utils.DeleteObjectsFromYaml(installYamlPath, installYamlFilename, ndbOperatorResources, namespace)

	// wait for ndb operator deployment to disappear
	ginkgo.By(fmt.Sprintf("Waiting for ndb-operator deployment to disappear"))
	err := deployment_utils.WaitForDeploymentToDisappear(clientset, namespace, "ndb-operator")
	framework.ExpectNoError(err)
}
