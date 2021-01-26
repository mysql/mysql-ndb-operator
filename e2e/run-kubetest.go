// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// Tool to run end to end tests using Kubetest2 and Ginkgo

//+build ignore

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const (
	// Kubetest2 version used by the tool. It doesn't have
	// release tags - so a known working version is used
	KUBETEST2_VERSION = "43699a7ba2"
	// KinD version used by the testing
	KIND_VERSION = "v0.9.0"
	// K8s image used by KinD to bring up cluster
	// The k8s 1.18.2 image built for KinD 0.8.0 is used
	// https://github.com/kubernetes-sigs/kind/releases/tag/v0.8.0
	KIND_K8S_IMAGE = "kindest/node:v1.18.2@sha256:7b27a6d0f2517ff88ba444025beae41491b016bc6af573ba467b70c5e8e0d85f"
)

// Command line options
var options struct {
	useKind    bool
	kubeconfig string
}

// assertDockerRunning checks that docker is running. It doesn't
// do anything if docker is running but if it is not, the program exits.
func assertDockerRunning(t *testRunner) {
	if err := t.execCommand(true, "docker", "info"); err != nil {
		log.Fatalf("❌ Error accessing docker : %s.\n"+
			"Please ensure that docker daemon is running and accessible.", err)
	}
	log.Printf(" 🐳 Docker daemon detected and accessible!")
}

// validateKubeConfig validates the config passed to --kubeconfig
// and verifies that the K8s cluster is running
func validateKubeConfig(kubeconfig string) bool {
	// Read the config from kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Printf("❌ Error loading kubeconfig from '%s': %s\n", kubeconfig, err)
		return false
	}

	// Create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("❌ Error creating clientset from kubeconfig at '%s': %s\n", kubeconfig, err)
		return false
	}

	// Retrieve version and verify
	var version *version.Info
	if version, err = clientset.ServerVersion(); err != nil {
		log.Printf("❌ Error finding out the version of the K8s cluster : %s\n", err)
		return false
	}

	log.Printf(" 😊 Successfully validated kubeconfig. Kubernetes Server version : %s\n", version.String())
	return true
}

// provider is an interface for the k8s cluster providers
type provider interface {
	init(t *testRunner)
	getKubeConfig(t *testRunner) string
	getKubeTestArgs(t *testRunner) []string
}

// kind implements provider using KinD
type kind struct{}

// newKindProvider returns a new kind provider
func newKindProvider() *kind {
	log.Println("🔧 Configuring kubetest with KinD")
	return &kind{}
}

// init initializes the kind provider
func (k *kind) init(t *testRunner) {
	// Ensure that docker is running
	assertDockerRunning(t)

	// Ensure that the right version of kubetest2-kind and KinD are installed
	t.goInstall("sigs.k8s.io/kubetest2/kubetest2-kind@" + KUBETEST2_VERSION)
	t.goInstall("sigs.k8s.io/kind@" + KIND_VERSION)
}

// getKubeConfig returns the Kubeconfig to connect to the cluster
func (k *kind) getKubeConfig(t *testRunner) string {
	return filepath.Join(t.testDir, "artifacts", "kubeconfig")
}

// getKubeTestArgs returns the kind specific arguments to be used with kubetest
func (k *kind) getKubeTestArgs(t *testRunner) []string {
	return []string{
		"kind",
		"--up", "--down",
		"--cluster-name=ndb-e2e-test",
		"--image-name=" + KIND_K8S_IMAGE,
		"--kubeconfig=" + k.getKubeConfig(t),
		// By default bring up a 3 node cluster - 1 control plane + 2 workers
		"--config=" + filepath.Join(t.testDir, "config", "kind-3-node-cluster.yaml"),
	}
}

// local implements provider using to run tests in a existing k8s cluster
type local struct {
	// kubeconfig is the kubeconfig of the cluster
	kubeconfig string
}

// newLocalProvider returns a new local provider
func newLocalProvider() *local {
	log.Println("🔧 Configuring kubetest to run on an existing cluster")
	return &local{}
}

// init initializes the local provider
func (l *local) init(t *testRunner) {
	// Ensure that the right version of kubetest2-kind and KinD are installed
	// KinD is used as a dummy/skeleton provider for local - it actually
	// doesn't do anything other than satisfying kubetest2's argument needs.
	t.goInstall("sigs.k8s.io/kubetest2/kubetest2-kind@" + KUBETEST2_VERSION)

	// Validate the kubeconfig
	if len(options.kubeconfig) > 0 {
		if validateKubeConfig(options.kubeconfig) {
			l.kubeconfig = options.kubeconfig
			return
		}
	}

	log.Fatalf("⚠️  Please pass a valid kubeconfig")
}

// getKubeConfig returns the Kubeconfig to connect to the cluster
func (l *local) getKubeConfig(t *testRunner) string {
	return l.kubeconfig
}

// getKubeTestArgs returns the kubetest args for local provider
func (l *local) getKubeTestArgs(t *testRunner) []string {
	// Nothing to do. Just choose kind provider to use as dummy
	return []string{"kind"}
}

// testRunner is the struct used to run kubetest
type testRunner struct {
	// testDir is the absolute path of e2e test directory
	testDir string
}

// init sets up the the testRunner
func (t *testRunner) init() {
	// Update log to print only line numbers
	log.SetFlags(log.Lshortfile)

	// Deduce test root directory
	_, currentFilePath, _, _ := runtime.Caller(0)
	t.testDir = filepath.Dir(currentFilePath)

	// Ensure that the right version of kubetest2 and the exec tester exist
	t.goInstall("sigs.k8s.io/kubetest2@" + KUBETEST2_VERSION)
	t.goInstall("sigs.k8s.io/kubetest2/kubetest2-tester-exec@" + KUBETEST2_VERSION)
}

// buildCommand constructs a cmd struct for the given command and arguments
func (t *testRunner) buildCommand(quiet bool, commandName string, commandArgs ...string) *exec.Cmd {
	// Create cmd struct
	cmd := exec.Command(commandName, commandArgs...)

	// Map the stout and stderr if not quiet
	if !quiet {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	// Return command
	return cmd
}

// execCommand executes the given command
func (t *testRunner) execCommand(quiet bool, commandName string, commandArgs ...string) error {
	return t.buildCommand(quiet, commandName, commandArgs...).Run()
}

// goInstall runs go get to install the requested executable
func (t *testRunner) goInstall(executable string) {
	log.Printf("📥 go get %s", executable)
	cmd := t.buildCommand(false, "go", "get", executable)
	// Execute in a directory outside project to prevent go.mod from being updated
	cmd.Dir = "/tmp"
	// Turn on GO111MODULE env to enable go get particular versions
	cmd.Env = append(os.Environ(), "GO111MODULE=on")

	if err := cmd.Run(); err != nil {
		log.Fatalf("❌ go get %s [failed]", executable)
	}

	log.Printf("✅ go get %s [done]\n", executable)
}

// run executes the e2e test using kubetest2
func (t *testRunner) run() {

	var p provider

	if options.useKind {
		p = newKindProvider()
	} else {
		p = newLocalProvider()
	}

	// init provider
	p.init(t)
	// Fetch provider specific arguments
	kubetestArgs := p.getKubeTestArgs(t)
	// Add any generic provider arguments
	kubetestArgs = append(kubetestArgs, "--artifacts="+filepath.Join(t.testDir, "artifacts"))

	// Add arguments to run gingko
	// kubetest2 ... --test=exec -- <ginkgo cmd + args> -- <test arguments>
	kubetestArgs = append(kubetestArgs, "--test=exec",
		// Args to run gingko
		"--",
		"go", "run", "github.com/onsi/ginkgo/ginkgo", "e2e",
		// Args to pass to the tests
		"--",
		"--kubeconfig="+p.getKubeConfig(t))

	// Prepare kubetest command
	// Get GOBIN location
	var goBinDir string
	var ok bool
	if goBinDir, ok = os.LookupEnv("GOBIN"); !ok {
		goBinDir = filepath.Join(os.Getenv("GOPATH"), "bin")
	}

	// Get absolute path to kubetest2 to pass to Command as GOBIN might not be in PATH
	kubetestCmdPath := filepath.Join(goBinDir, "kubetest2")

	// Construct the kubetestCmd struct with args
	kubetestCmd := t.buildCommand(false, kubetestCmdPath, kubetestArgs...)

	// Update kubetestCmd's env PATH to include GOBIN
	updatedPath := fmt.Sprintf("PATH=%s%c%s", os.Getenv("PATH"), filepath.ListSeparator, goBinDir)
	kubetestCmd.Env = append(os.Environ(), updatedPath)

	// Run kubetest2
	log.Println("🔨 Running kubetest : kubetest2 " + strings.Join(kubetestArgs, " "))
	if err := kubetestCmd.Run(); err != nil {
		log.Fatalf("❌ Kubetest failed : %s", err)
	}
}

func init() {

	flag.BoolVar(&options.useKind, "use-kind", false,
		"Use KinD to run the e2e tests. By default, this is disabled and the tests will be run in an existing K8s cluster.")

	var defaultKubeconfig string
	if home := homedir.HomeDir(); home != "" {
		// use kubeconfig at $HOME/.kube/config as the default
		defaultKubeconfig = filepath.Join(home, ".kube", "config")
	}
	flag.StringVar(&options.kubeconfig, "kubeconfig", defaultKubeconfig,
		"Kubeconfig of the existing K8s cluster to run tests on. This will not be used if '--use-kind' is enabled.")
}

func main() {
	flag.Parse()
	t := testRunner{}
	t.init()
	t.run()
}
