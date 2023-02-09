// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapiutils

import (
	"fmt"

	"github.com/mysql/ndb-operator/e2e-tests/utils/service"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/mgmapi"
	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
)

// ConnectToMgmd connects to a Management Server of the MySQL Cluster
// represented by the ndb object and returns a mgmapi.MgmClient
func ConnectToMgmd(clientset kubernetes.Interface, ndb *v1.NdbCluster) mgmapi.MgmClient {
	ginkgo.By("connecting to the Management Node service")
	serviceName := ndb.GetServiceName("mgmd")
	host, port := service.GetServiceAddressAndPort(clientset, ndb.Namespace, serviceName)
	mgmClient, err := mgmapi.NewMgmClient(fmt.Sprintf("%s:%d", host, port))
	gomega.Expect(err).Should(gomega.Succeed())
	return mgmClient
}

// ForEachConnectedNodes runs the given function for
// every connected MySQL Cluster node of type nodeType
func ForEachConnectedNodes(
	clientset kubernetes.Interface, ndb *v1.NdbCluster, nodeType mgmapi.NodeTypeEnum,
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

// ExpectConfigVersionInMySQLClusterNodes checks if the MySQL Cluster
// nodes run with the expected config version.
func ExpectConfigVersionInMySQLClusterNodes(
	c kubernetes.Interface, testNdb *v1.NdbCluster, expectedConfigVersion uint32) {
	ginkgo.By(
		fmt.Sprintf("expecting config version of MySQL Cluster nodes to be %d", expectedConfigVersion))

	// Check config of connected mgmd node
	client := ConnectToMgmd(c, testNdb)
	defer client.Disconnect()
	gomega.Expect(client.GetConfigVersion()).To(gomega.Equal(expectedConfigVersion))

	// Check config of all data nodes
	ForEachConnectedNodes(c, testNdb, mgmapi.NodeTypeNDB, func(mgmClient mgmapi.MgmClient, nodeId int) {
		gomega.Expect(mgmClient.GetConfigVersion(nodeId)).To(gomega.Equal(expectedConfigVersion))
	})
}
