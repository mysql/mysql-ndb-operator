// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import (
	"fmt"
	"github.com/mysql/ndb-operator/e2e-tests/utils/service"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/mgmapi"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
)

// ConnectToMgmd connects to a Management Server of the MySQL Cluster
// represented by the ndb object and returns a mgmapi.MgmClient
func ConnectToMgmd(clientset kubernetes.Interface, ndb *v1alpha1.NdbCluster) mgmapi.MgmClient {
	ginkgo.By("connecting to the Management Node service")
	serviceName := ndb.GetServiceName("mgmd") + "-ext"
	host, port := service.GetServiceAddressAndPort(clientset, ndb.Namespace, serviceName)
	mgmClient, err := mgmapi.NewMgmClient(fmt.Sprintf("%s:%d", host, port))
	gomega.Expect(err).Should(gomega.Succeed())
	return mgmClient
}

// ForEachConnectedNodes runs the given function for
// every connected MySQL Cluster node of type nodeType
func ForEachConnectedNodes(
	clientset kubernetes.Interface, ndb *v1alpha1.NdbCluster, nodeType mgmapi.NodeTypeEnum,
	testFunc func(mgmClient mgmapi.MgmClient, nodeId int)) {
	// connect to MySQL Cluster
	mgmClient := ConnectToMgmd(clientset, ndb)
	defer mgmClient.Disconnect()

	// Get Cluster Status
	cs, err := mgmClient.GetStatus()
	gomega.Expect(err).Should(gomega.Succeed())
	// Run the test on desired nodes
	for _, node := range cs {
		if node.IsConnected && node.NodeType == nodeType {
			testFunc(mgmClient, node.NodeId)
		}
	}
}
