// Copyright (c) 2022, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package e2e

import (
	"context"
	"net"

	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/mysql/ndb-operator/e2e-tests/utils/ndbtest"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
	"github.com/mysql/ndb-operator/pkg/mgmapi"
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

func testMgmdConnection(mgmdHost string) {
	connectString := mgmdHost + ":1186"
	mgmClient, err := mgmapi.NewMgmClient(connectString)
	gomega.Expect(err).Should(gomega.Succeed(),
		"failed to connect to management server at %s", connectString)
	_, err = mgmClient.GetStatus()
	ndbtest.ExpectNoError(err, "GetStatus failed")
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

		ginkgo.It("should be able to access mgmd via the service", func() {
			serviceName := testNdb.GetServiceName(constants.NdbNodeTypeMgmd) + "." + testNdb.Namespace
			testMgmdConnection(serviceName)
		})
	})

	ginkgo.When("the Management Service LoadBalancer is enabled", func() {
		ginkgo.It("should not affect the service to the Management nodes and its IPs", func() {
			// Run GetStatus in parallel via service and management pod IP.
			serviceName := testNdb.GetServiceName("mgmd") + "." + testNdb.Namespace

			// Record the IPs of the service before updating NdbCluster
			ipBeforeUpdate, err := net.LookupIP(serviceName)
			ndbtest.ExpectNoError(err, "unable to lookup, invalid_host")
			gomega.Expect(ipBeforeUpdate).NotTo(gomega.BeNil())

			// Ip/Services to be tested during service update
			connectStrings := []string{
				serviceName, ipBeforeUpdate[0].String(),
			}

			stop := make(chan bool, 2)
			for i := range connectStrings {
				// Run GetStatus in parallel via service and management pod IP
				// to verify that the service is not affected during patch.
				connectString := connectStrings[i]
				go func() {
					defer ginkgo.GinkgoRecover()
					for {
						select {
						default:
							testMgmdConnection(connectString)
						case <-stop:
							return
						}
					}
				}()

				defer func() {
					stop <- true
				}()
			}

			// Enable Management Load Balancer
			testNdb.Spec.ManagementNode.EnableLoadBalancer = true
			ndbtest.KubectlApplyNdbObj(c, testNdb)

			// Get the IPs of the service after updating NdbCluster
			ipAfterUpdate, err := net.LookupIP(serviceName)
			ndbtest.ExpectNoError(err, "unable to lookup, invalid_host")
			gomega.Expect(ipAfterUpdate).NotTo(gomega.BeNil())

			gomega.Expect(ipBeforeUpdate).To(
				gomega.Equal(ipAfterUpdate), "Management IP changed during service patch request")
		})

		ginkgo.It("should create LoadBalancer services for Management Server", func() {
			expectServiceType(ctx, svcInterface, testNdb, constants.NdbNodeTypeMgmd, corev1.ServiceTypeLoadBalancer)
		})

		ginkgo.It("should create ClusterIP services for MySQL Servers", func() {
			expectServiceType(ctx, svcInterface, testNdb, constants.NdbNodeTypeMySQLD, corev1.ServiceTypeClusterIP)
		})

		ginkgo.It("should create headless service for Data Nodes", func() {
			expectServiceType(ctx, svcInterface, testNdb, constants.NdbNodeTypeNdbmtd, ServiceTypeHeadless)
		})
	})

	ginkgo.When("the MySQL Server LoadBalancer is enabled", func() {
		ginkgo.BeforeAll(func() {
			// Enable MySQL Load Balancer
			testNdb.Spec.MysqlNode.EnableLoadBalancer = true
			ndbtest.KubectlApplyNdbObj(c, testNdb)
		})

		ginkgo.It("should create LoadBalancer services for Management and MySQL Server", func() {
			expectServiceType(ctx, svcInterface, testNdb, constants.NdbNodeTypeMgmd, corev1.ServiceTypeLoadBalancer)
			expectServiceType(ctx, svcInterface, testNdb, constants.NdbNodeTypeMySQLD, corev1.ServiceTypeLoadBalancer)
		})

		ginkgo.It("should create headless service for Data Nodes", func() {
			expectServiceType(ctx, svcInterface, testNdb, constants.NdbNodeTypeNdbmtd, ServiceTypeHeadless)
		})
	})
})
