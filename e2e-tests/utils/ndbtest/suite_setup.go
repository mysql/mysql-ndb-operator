// Defines common setup methods for all e2e test suites

package ndbtest

import (
	"flag"
	"github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	"github.com/onsi/gomega"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/testfiles"
)

func init() {
	// klog arguments
	klog.InitFlags(nil)

	// framework args
	// we don't use framework.RegisterCommonFlags() framework.RegisterClusterFlags() yet
	flag.StringVar(&framework.TestContext.KubeConfig,
		"kubeconfig", "", "Kubeconfig of the existing K8s cluster to run tests on.\n"+
			"Only required if running from outside the K8s CLuster.")
	flag.StringVar(&framework.TestContext.KubectlPath,
		"kubectl-path", "kubectl", "The kubectl binary to use. For development, you might use 'cluster/kubectl.sh' here.")
}

// suiteConfig holds the common configs of a suite
var suiteConfig struct {
	suiteSetupDone bool
	name           string
	f              *framework.Framework
}

// GetFramework returns the common framework used by the suite
// This should be called only inside a child ginkgo node so
// that the namespace and clientset will be set up
func GetFramework() *framework.Framework {
	f := suiteConfig.f
	// ensure that the framework is created
	gomega.Expect(f).NotTo(gomega.BeNil())
	if f.Namespace == nil || f.ClientSet == nil {
		// GetFramework should be called only from a child node
		// so that the namespace and clientset are setup when
		// the framework's BeforeEach is run. Die here to
		// prevent misuse of this method during development.
		klog.Fatal("GetFramework() should be called only from a ginkgo child node")
	}
	return suiteConfig.f
}

// RunGinkgoSuite sets up a ginkgo suite, the e2e framework
// and then starts running the test specs. This needs to be
// called only once from a suite inside a golang test
// method to start the tests
// example :
//  func Test_NdbBasic(t *testing.T) {
//	  ndbtest.RunGinkgoSuite(t, "ndb-basic", "Ndb operator basic", true)
//  }
func RunGinkgoSuite(t *testing.T, suiteName string, description string, createFramework bool) {
	if suiteConfig.suiteSetupDone {
		// RunGinkgoSuite called again in a suite. Exit after reporting error.
		t.Helper()
		t.Fatal("RunGinkgoSuite should be called only once within a suite")
	}

	if validation.IsDNS1123Label(suiteName) != nil {
		t.Helper()
		t.Fatal("Suite name should be a valid DNS1123Label as it will be used to create namespaces")
	}
	suiteConfig.name = suiteName

	flag.Parse()

	// Fail if operator image is not found
	//image_utils.CheckOperatorImage()
	// Add the repo root as a file source
	testfiles.AddFileSource(testfiles.RootFileSource{Root: "../../.."})

	// Ndb operator tests can take more than the default
	// 5 secs to complete. To avoid getting marked as slow,
	// update the default slow spec threshold to 2 minutes.
	config.DefaultReporterConfig.SlowSpecThreshold = 2 * time.Minute.Seconds()
	// disable succinct report option to get a mini detailed report
	config.DefaultReporterConfig.Succinct = false

	if createFramework {
		// set framework options
		framework.TestContext.Provider = "local"
		framework.TestContext.DeleteNamespace = true
		framework.TestContext.DeleteNamespaceOnFailure = true
		framework.TestContext.GatherKubeSystemResourceUsageData = "none"
		framework.TestContext.GatherMetricsAfterTest = "false"
		framework.AfterReadingAllFlags(&framework.TestContext)

		// create a new e2e framework for the suite.
		// This will register the following methods :
		//  - A BeforeEach that creates a new namespace before
		//    running every spec and
		//  - An AfterEach that deletes the namespace and
		//    clientset after the spec is run
		//
		// The namespace and clientset will be available in the
		// framework object variables only when the spec is being
		// run, so they should be used only inside the child
		// ginkgo nodes (i.e It, When, BeforeEach, AfterEach etc.)
		//
		// This framework is created before any Describe blocks
		// are defined to ensure that the BeforeEach and
		// AfterEach registered will the first and last to be
		// called respectively when a spec is run.
		suiteConfig.f = framework.NewDefaultFramework(suiteName)
	}

	suiteConfig.suiteSetupDone = true

	// Register fail handler and start running the test
	gomega.RegisterFailHandler(framework.Fail)
	ginkgo.RunSpecs(t, description)
}
