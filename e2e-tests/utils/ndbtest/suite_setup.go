// Defines common setup methods for all e2e test suites

package ndbtest

import (
	"flag"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"k8s.io/klog"
	"os"
	"path/filepath"
	"testing"

	image_utils "github.com/mysql/ndb-operator/e2e-tests/utils/image"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/testfiles"
)

func init() {
	// klog arguments
	klog.InitFlags(nil)

	// framework args
	// we don't use framework.RegisterCommonFlags() framework.RegisterClusterFlags() yet
	// use kubeconfig at $HOME/.kube/config as the default
	defaultKubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	flag.StringVar(&framework.TestContext.KubeConfig,
		"kubeconfig", defaultKubeconfig, "Path to a kubeconfig.")
	flag.StringVar(&framework.TestContext.KubectlPath,
		"kubectl-path", "kubectl", "The kubectl binary to use. For development, you might use 'cluster/kubectl.sh' here.")
}

// RunGinkgoSuite sets up a ginkgo suite, the e2e framework
// and then starts running the test specs. This needs to be
// called only once from a suite inside a golang test
// method to start the tests
// example :
//  func Test_NdbBasic(t *testing.T) {
//	  ndbtest.RunGinkgoSuite(t, "Ndb Suite")
//  }
func RunGinkgoSuite(t *testing.T, description string) {
	flag.Parse()

	// Fail if operator image is not found
	image_utils.CheckOperatorImage()

	// set framework options
	framework.TestContext.Provider = "local"
	framework.TestContext.DeleteNamespace = true
	framework.TestContext.DeleteNamespaceOnFailure = true
	framework.TestContext.RepoRoot = "../../.."
	framework.TestContext.GatherKubeSystemResourceUsageData = "none"
	framework.TestContext.GatherMetricsAfterTest = "false"
	framework.AfterReadingAllFlags(&framework.TestContext)
	testfiles.AddFileSource(testfiles.RootFileSource{Root: framework.TestContext.RepoRoot})

	// Register fail handler and start running the test
	gomega.RegisterFailHandler(framework.Fail)
	ginkgo.RunSpecs(t, description)
}
