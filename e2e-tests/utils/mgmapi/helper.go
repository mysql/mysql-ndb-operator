package mgmapi

import (
	"github.com/mysql/ndb-operator/e2e-tests/utils/service"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/mgmapi"
	"github.com/onsi/ginkgo"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

// ConnectToMgmd connects to a Management Server of the MySQL Cluster
// represented by the ndb object and returns a mgmapi.MgmClient
func ConnectToMgmd(clientset kubernetes.Interface, ndb *v1alpha1.Ndb) mgmapi.MgmClient {
	ginkgo.By("connecting to the Management Node service")
	serviceName := ndb.GetServiceName("mgmd") + "-ext"
	connectstring := service.GetExternalIP(clientset, ndb.Namespace, serviceName)
	mgmClient, err := mgmapi.NewMgmClient(connectstring + ":1186")
	framework.ExpectNoError(err)
	return mgmClient
}

// ForEachConnectedNodes runs the given function for
// every connected MySQL Cluster node of type nodeType
func ForEachConnectedNodes(
	clientset kubernetes.Interface, ndb *v1alpha1.Ndb, nodeType mgmapi.NodeTypeEnum,
	testFunc func(mgmClient mgmapi.MgmClient, nodeId int)) {
	// connect to MySQL Cluster
	mgmClient := ConnectToMgmd(clientset, ndb)
	defer mgmClient.Disconnect()

	// Get Cluster Status
	cs, err := mgmClient.GetStatus()
	framework.ExpectNoError(err)
	// Run the test on desired nodes
	for _, node := range *cs {
		if node.IsConnected && node.NodeType == nodeType {
			testFunc(mgmClient, node.NodeId)
		}
	}
}
