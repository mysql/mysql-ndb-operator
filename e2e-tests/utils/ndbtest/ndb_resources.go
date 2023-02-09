// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbtest

import (
	event_utils "github.com/mysql/ndb-operator/e2e-tests/utils/events"
	sfset_utils "github.com/mysql/ndb-operator/e2e-tests/utils/statefulset"
	yaml_utils "github.com/mysql/ndb-operator/e2e-tests/utils/yaml"

	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/controllers"

	ginkgo "github.com/onsi/ginkgo/v2"
	"k8s.io/client-go/kubernetes"
	klog "k8s.io/klog/v2"
)

// waitForNdbSync waits for a ReasonSyncSuccess event to occur
func waitForNdbSync(clientset kubernetes.Interface, namespace string, lastKnownEventResourceVersion string) {
	err := event_utils.WaitForEvent(clientset, namespace, controllers.ReasonSyncSuccess, lastKnownEventResourceVersion)
	ExpectNoError(err, "timed out waiting for operator to send sync event")
}

func waitForNdbDelete(clientset kubernetes.Interface, namespace, ndbName string) {
	err := sfset_utils.WaitForStatefulSetToDisappear(clientset, namespace, ndbName+"-"+constants.NdbNodeTypeNdbmtd)
	ExpectNoError(err, "timed out waiting for ndb statefulset to disappear")
	err = sfset_utils.WaitForStatefulSetToDisappear(clientset, namespace, ndbName+"-"+constants.NdbNodeTypeMgmd)
	ExpectNoError(err, "timed out waiting for mgmd statefulset to disappear")
	err = sfset_utils.WaitForStatefulSetToDisappear(clientset, namespace, ndbName+"-"+constants.NdbNodeTypeMySQLD)
	ExpectNoError(err, "timed out waiting for mysqld deployment to disappear")
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

// KubectlApplyNdbObjNoWait creates a Ndb K8s object from the given Ndb object
// and returns without waiting for the MySQL Cluster to become ready
func KubectlApplyNdbObjNoWait(ndb *v1.NdbCluster) {
	klog.V(2).Infof("creating/updating Ndb resource from %s", ndb.Namespace)
	yamlContent := yaml_utils.MarshalNdb(ndb)
	ginkgo.By("creating/updating the Ndb resource")
	RunKubectl(ApplyCmd, ndb.Namespace, string(yamlContent))
}

// KubectlApplyNdbObj creates a Ndb K8s object from the given Ndb object
// and waits for the ndb operator to setup the MySQL Cluster
func KubectlApplyNdbObj(clientset kubernetes.Interface, ndb *v1.NdbCluster) {
	lastKnownEventResourceVersion := event_utils.GetLastKnownEventResourceVersion(clientset, ndb.Namespace)
	KubectlApplyNdbObjNoWait(ndb)
	waitForNdbSync(clientset, ndb.Namespace, lastKnownEventResourceVersion)
}

// KubectlDeleteNdbObj deletes an Ndb K8s object from the given Ndb object
// and waits for MySQL Cluster to shutdown
func KubectlDeleteNdbObj(clientset kubernetes.Interface, ndb *v1.NdbCluster) {
	klog.V(2).Infof("Deleting Ndb resource from %s", ndb.Namespace)

	yamlContent := yaml_utils.MarshalNdb(ndb)

	ginkgo.By("deleting the Ndb resource and waiting for the workloads to disappear")
	RunKubectl(DeleteCmd, ndb.Namespace, string(yamlContent))
	waitForNdbDelete(clientset, ndb.Namespace, ndb.Name)
}
