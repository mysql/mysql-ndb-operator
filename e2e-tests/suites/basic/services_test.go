// Copyright (c) 2022, 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"

	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/mysql/ndb-operator/e2e-tests/utils/k8sutils"
	mgmapiutils "github.com/mysql/ndb-operator/e2e-tests/utils/mgmapi"
	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	networkpolicyutils "github.com/mysql/ndb-operator/e2e-tests/utils/networkpolicy"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
)

const ServiceTypeHeadless = "Headless"

func expectServiceType(ctx context.Context, svcInterface typedcorev1.ServiceInterface,
	testNdb *v1.NdbCluster, resource string, serviceType corev1.ServiceType) {
	serviceName := testNdb.GetServiceName(resource)
	svc, err := svcInterface.Get(ctx, serviceName, metav1.GetOptions{})
	ndbtest.ExpectNoError(err, "Get Service %q failed", serviceName)
	if serviceType == ServiceTypeHeadless {
		gomega.Expect(svc.Spec.Type).To(gomega.Equal(corev1.ServiceTypeClusterIP))
		gomega.Expect(svc.Spec.ClusterIP).To(gomega.Equal(corev1.ClusterIPNone))
	} else {
		gomega.Expect(svc.Spec.Type).To(gomega.Equal(serviceType))
	}
}

var _ = ndbtest.NewOrderedTestCase("NdbCluster Services", func(tc *ndbtest.TestContext) {
	var ns string
	var c clientset.Interface
	var testNdb *v1.NdbCluster
	var ctx context.Context
	var ndbName string
	var svcInterface typedcorev1.ServiceInterface

	ginkgo.BeforeAll(func() {
		ginkgo.By("extracting values from TestContext")
		ns = tc.Namespace()
		c = tc.K8sClientset()
		ctx = tc.Ctx()
		svcInterface = c.CoreV1().Services(ns)

		// Create the NdbCluster object to be used by the testcases
		ndbName = "services-test"
		testNdb = testutils.NewTestNdb(ns, ndbName, 2)

		ndbtest.KubectlApplyNdbObj(c, testNdb)

		// Setup cleanup methods
		ginkgo.DeferCleanup(func() {
			// Common cleanup for all specs cleanup
			ndbtest.KubectlDeleteNdbObj(c, testNdb)
		})

	})

	ginkgo.When("the NdbCluster is created with default field values", func() {
		ginkgo.It("should create ClusterIP services for Management and MySQL Servers", func() {
			expectServiceType(ctx, svcInterface, testNdb, constants.NdbNodeTypeMgmd, corev1.ServiceTypeClusterIP)
			expectServiceType(ctx, svcInterface, testNdb, constants.NdbNodeTypeMySQLD, corev1.ServiceTypeClusterIP)
		})

		ginkgo.It("should create headless service for Data Nodes", func() {
			expectServiceType(ctx, svcInterface, testNdb, constants.NdbNodeTypeNdbmtd, ServiceTypeHeadless)
		})

		ginkgo.It("should allow explicitly labeled same-namespace pods to access mgmd via the service", func() {
			mgmapiutils.ExpectCanConnectToMgmd(c, testNdb)
		})

		ginkgo.It("should require NetworkPolicy enforcement from the cluster CNI", func() {
			gomega.Expect(networkpolicyutils.IsNetworkPolicyEnforced(ctx, c, ns)).To(gomega.BeTrue(),
				"the E2E environment must enforce networking.k8s.io/v1 NetworkPolicy")
		})

		ginkgo.It("should block unlabeled same-namespace pods from accessing mgmd", func() {
			mgmapiutils.ExpectCannotConnectToMgmd(c, testNdb)
		})

		ginkgo.It("should block labeled pods from another namespace", func() {
			// Spoof every allowed pod label to prove the namespace is the trust boundary.
			externalNamespace := ns + "-mgmd-client"
			_, err := k8sutils.CreateNamespace(ctx, c, externalNamespace)
			ndbtest.ExpectNoError(err, "failed to create cross-namespace mgmd client namespace")
			defer func() {
				err := k8sutils.DeleteNamespace(ctx, c, externalNamespace)
				ndbtest.ExpectNoError(err, "failed to delete cross-namespace mgmd client namespace")
			}()

			mgmapiutils.ExpectCannotConnectToMgmdFromNamespace(c, testNdb, externalNamespace)
		})
	})

	ginkgo.When("the MySQL Server LoadBalancer is enabled", func() {
		ginkgo.BeforeAll(func() {
			// Enable MySQL Load Balancer
			testNdb.Spec.MysqlNode.EnableLoadBalancer = true
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.It("should keep Management Server ClusterIP and create LoadBalancer service for MySQL Server", func() {
			expectServiceType(ctx, svcInterface, testNdb, constants.NdbNodeTypeMgmd, corev1.ServiceTypeClusterIP)
			expectServiceType(ctx, svcInterface, testNdb, constants.NdbNodeTypeMySQLD, corev1.ServiceTypeLoadBalancer)
		})

		ginkgo.It("should create headless service for Data Nodes", func() {
			expectServiceType(ctx, svcInterface, testNdb, constants.NdbNodeTypeNdbmtd, ServiceTypeHeadless)
		})
	})
})
