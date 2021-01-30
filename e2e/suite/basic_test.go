// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// NOTE: NOT READY AT ALL - FIX BUT DON'T USE

package e2e

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"

	"k8s.io/component-base/version"
	"k8s.io/klog"

	"k8s.io/kubernetes/test/e2e/framework"

	"k8s.io/kubernetes/test/e2e/framework/testfiles"
	"k8s.io/kubernetes/test/utils/image"
)

var (
	versionFlag   bool
	listProviders bool
)

func TestMain(t *testing.T) {

	klog.InitFlags(nil)

	flag.Parse()

	if listProviders {
		klog.Infof("Listing providers.")
		for _, p := range framework.GetProviders() {
			fmt.Println(p)
		}
		os.Exit(0)
	}

	if framework.TestContext.ListImages {
		fmt.Println("Listing images:")
		for _, v := range image.GetImageConfigs() {
			fmt.Println(v.GetE2EImage())
		}
		os.Exit(0)
	}
	if versionFlag {
		fmt.Printf("%s\n", version.Get())
		os.Exit(0)
	}

	framework.TestContext.Provider = "local"
	framework.TestContext.DeleteNamespace = true
	framework.TestContext.DeleteNamespaceOnFailure = true
	framework.TestContext.RepoRoot = "../../"

	framework.AfterReadingAllFlags(&framework.TestContext)

	testfiles.AddFileSource(testfiles.RootFileSource{Root: framework.TestContext.RepoRoot})
}

func Test_NdbBasic(t *testing.T) {
	fmt.Printf("%s\n", "Starting to test ...")
	gomega.RegisterFailHandler(framework.Fail)
	ginkgo.RunSpecs(t, "Ndb Suite")
}

// we don't use framework.RegisterCommonFlags() framework.RegisterClusterFlags() yet
func init() {
	flag.StringVar(&framework.TestContext.KubeConfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")

	flag.StringVar(&framework.TestContext.KubectlPath, "kubectl-path", "kubectl", "The kubectl binary to use. For development, you might use 'cluster/kubectl.sh' here.")

	flag.BoolVar(&framework.TestContext.ListImages, "listimages", false, "List images.")

	flag.BoolVar(&versionFlag, "version", false, "Show version information.")
	flag.BoolVar(&listProviders, "listproviders", false, "Show provider information.")
}
