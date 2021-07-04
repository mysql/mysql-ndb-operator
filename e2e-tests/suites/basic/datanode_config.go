// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"fmt"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"github.com/mysql/ndb-operator/pkg/mgmapi"

	mgmapi_utils "github.com/mysql/ndb-operator/e2e-tests/utils/mgmapi"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"

	clientset "k8s.io/client-go/kubernetes"
)

var _ = ndbtest.DescribeFeature("Datanode configuration", func() {
	var ns string
	var c clientset.Interface
	var testNdb *v1alpha1.Ndb

	ginkgo.BeforeEach(func() {
		ginkgo.By("extracting values from framework")
		f := ndbtest.GetFramework()
		ns = f.Namespace.Name
		c = f.ClientSet

		ginkgo.By(fmt.Sprintf("Deploying operator in namespace '%s'", ns))
		ndbtest.DeployNdbOperator(c, ns)
	})

	ginkgo.AfterEach(func() {
		ginkgo.By("Deleting mgmclient operator and other resources")
		ndbtest.UndeployNdbOperator(c, ns)
	})

	ginkgo.When("the data memory is specified in Ndb resource", func() {

		ginkgo.BeforeEach(func() {
			testNdb = testutils.NewTestNdb(ns, "mgmclient-datamemory-test", 2)
			// Set a 200M memory to the mgmclient resource and test if its set
			testNdb.Spec.DataMemory = "200M"
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.AfterEach(func() {
			ndbtest.KubectlDeleteNdbObj(c, testNdb)
		})

		ginkgo.It("should reflect in the datanodes' config", func() {
			// check if all the data nodes have the expected data memory
			ginkgo.By("checking if all the data nodes have the expected data memory")
			expectedDataMemory := uint64(200 * 1024 * 1024)
			mgmapi_utils.ForEachConnectedNodes(c, testNdb, mgmapi.NodeTypeNDB, func(mgmClient mgmapi.MgmClient, nodeId int) {
				gomega.Expect(mgmClient.GetDataMemory(nodeId)).To(gomega.Equal(expectedDataMemory))
			})

			ginkgo.By("updating the data memory in Ndb resource")
			testNdb.Spec.DataMemory = "300M"
			ndbtest.KubectlApplyNdbObj(c, testNdb)

			ginkgo.By("checking if all the data nodes have the new expected data memory")
			expectedDataMemory = uint64(300 * 1024 * 1024)
			mgmapi_utils.ForEachConnectedNodes(c, testNdb, mgmapi.NodeTypeNDB, func(mgmClient mgmapi.MgmClient, nodeId int) {
				gomega.Expect(mgmClient.GetDataMemory(nodeId)).To(gomega.Equal(expectedDataMemory))
			})
		})
	})

})
