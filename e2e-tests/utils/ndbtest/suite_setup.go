// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// Defines common setup methods for all e2e test suites

package ndbtest

import (
	"context"
	"flag"
	"os"
	"testing"

	ginkgo "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	ndbclient "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
	"github.com/mysql/ndb-operator/pkg/helpers"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	klog "k8s.io/klog/v2"
)

func init() {
	// klog arguments
	klog.InitFlags(nil)
	// Suppress logs to stderr as they will be sent to GinkgoWriter
	_ = flag.Set("logtostderr", "false")

	flag.StringVar(&ndbTestSuite.kubeConfig,
		"kubeconfig", "", "Kubeconfig of the existing K8s cluster to run tests on.\n"+
			"Only required if running from outside the K8s Cluster.")
	flag.StringVar(&ndbTestSuite.kubectlPath,
		"kubectl-path", "kubectl",
		"The kubectl binary to use. For development, you might use 'cluster/kubectl.sh' here.")
	flag.StringVar(&ndbTestSuite.ndbOperatorImage,
		"ndb-operator-image", "",
		"The NDB Operator image to be used by the e2e tests.\n"+
			"By default, the image specified in the helm chart will be used.")
}

// ndbTestSuite has all the information to run a single ginkgo test suite
var ndbTestSuite struct {
	t                *testing.T
	suiteName        string
	description      string
	clientset        kubernetes.Interface
	ndbClientset     ndbclient.Interface
	createNamespaces bool
	setupDone        bool
	// Command line arguments
	kubeConfig  string
	kubectlPath string
	// NDB operator image to be tested
	ndbOperatorImage string
	// channel through which the generated
	// unique Ids for a namespace name are sent
	uniqueId chan int
	// Context to be used by the test cases
	ctx context.Context
}

// newClientsets creates new kubernetes.Interface and
// ndbclient.Interface from the provided kubeconfig
func newClientsets(t *testing.T) (kubernetes.Interface, ndbclient.Interface) {
	runningInsideK8s := helpers.IsAppRunningInsideK8s()
	// K8s client configuration
	var cfg *restclient.Config
	var err error
	if runningInsideK8s {
		// Test is running inside K8s Pods
		cfg, err = restclient.InClusterConfig()
	} else {
		// Test is running outside K8s cluster
		cfg, err = clientcmd.BuildConfigFromFlags("", ndbTestSuite.kubeConfig)
	}
	if err != nil {
		t.Fatalf("Error getting kubeconfig: %s", err.Error())
	}

	// Create the general K8s Clientset
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	// Create the Ndb Clientset
	ndbClientset, err := ndbclient.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("Error building ndb clientset: %s", err.Error())
	}

	return clientset, ndbClientset
}

// RunGinkgoSuite sets up a ginkgo suite, and then starts
// running the test specs. This needs to be called only
// once from a suite inside a golang test method to start
// the tests. The list of CRD that needs to be installed
// before starting the suite can be passed via crdList.
// The CRD path needs to be relative to the project root
// directory.
// example :
//
//	 func Test_NdbBasic(t *testing.T) {
//		  framework.RunGinkgoSuite(t, "ndb-basic", "Ndb operator basic", true, true)
//	 }
func RunGinkgoSuite(
	t *testing.T, suiteName, description string,
	createClientsets, createNamespace bool,
	crdList []string) {
	if ndbTestSuite.setupDone {
		panic("RunGinkgoSuite should be called only once within a suite")
	}

	if validation.IsDNS1123Label(suiteName) != nil {
		panic("Suite name should be a valid DNS1123Label as it will be used to create namespaces")
	}

	// Skip running test if the test is running inside a pod
	// and user has aborted the previous suite.
	if helpers.IsAppRunningInsideK8s() {
		// The docker entrypoint script touches the /tmp/abort
		// file if the tests are being aborted.
		if _, err := os.Stat("/tmp/abort"); err == nil {
			t.Fatalf("Skipping suite %q as the tests are being aborted", suiteName)
		} else if !os.IsNotExist(err) {
			// Any other error than the IsNotFound should not occur
			panic("error occurred when trying to stat /tmp/abort : " + err.Error())
		}
	}

	// Redirect all klog output to GinkgoWriter
	klog.SetOutput(ginkgo.GinkgoWriter)

	// init ndbTestSuite
	ndbTestSuite.suiteName = suiteName
	ndbTestSuite.description = description
	ndbTestSuite.t = t

	if createNamespace {
		// Start uniqueId generator for namespace names
		// Namespace name will be of format <suiteName>-<uniqueId>
		ndbTestSuite.uniqueId = make(chan int)
		go func() {
			// generate unique IDs one by one forever.
			// This Go routine will stop when test exits.
			for i := 1; ; i++ {
				ndbTestSuite.uniqueId <- i
			}
		}()
		ndbTestSuite.createNamespaces = true
	}

	if createClientsets {
		// Create k8s clientsets
		ndbTestSuite.clientset, ndbTestSuite.ndbClientset = newClientsets(t)
	}

	// Register Before/AfterSuite methods
	setupBeforeAfterSuite(crdList)

	ndbTestSuite.ctx = context.Background()
	ndbTestSuite.setupDone = true

	// Update ginkgo config
	_, reporterConfig := ginkgo.GinkgoConfiguration()
	// disable succinct report option to get a mini detailed report
	reporterConfig.Succinct = false
	// Print stack trace on failure
	reporterConfig.FullTrace = true

	// Register fail handler and start running the test
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, description, reporterConfig)
}
