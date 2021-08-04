// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// Tool to run end to end tests using Kubetest2 and Ginkgo

//+build ignore

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Command line options
var options struct {
	useKind        bool
	kubeconfig     string
	inCluster      bool
	kindK8sVersion string
}

// K8s image used by KinD to bring up cluster
// https://github.com/kubernetes-sigs/kind/releases
var kindK8sNodeImages = map[string]string{
	"1.19": "kindest/node:v1.19.11@sha256:7664f21f9cb6ba2264437de0eb3fe99f201db7a3ac72329547ec4373ba5f5911",
	"1.20": "kindest/node:v1.20.7@sha256:e645428988191fc824529fd0bb5c94244c12401cf5f5ea3bd875eb0a787f0fe9",
}

var (
	kindCmd = []string{"go", "run", "sigs.k8s.io/kind"}
)

// buildKubernetesClientSetFromConfig builds a kubernetes clientset
// returns clientset if successfully built
func buildClientsetFromConfig(kubeconfig string) *kubernetes.Clientset {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Printf("❌ Error loading kubeconfig from '%s': %s", kubeconfig, err)
		return nil
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("❌ Error building kubernetes clientset: %s", err)
		return nil
	}
	return clientset
}

// validateKubeConfig validates the config passed to --kubeconfig
// and verifies that the K8s cluster is running
func validateKubeConfig(kubeconfig string) bool {
	// build clientset to verify k8sversion
	clientset := buildClientsetFromConfig(kubeconfig)
	if clientset == nil {
		return false
	}
	// Retrieve version and verify
	var k8sVersion *version.Info
	var err error
	if k8sVersion, err = clientset.ServerVersion(); err != nil {
		log.Printf(" ❌ Error finding out the version of the K8s cluster : %s", err)
		return false
	}

	log.Printf(" 👍 Successfully validated kubeconfig. Kubernetes Server version : %s", k8sVersion.String())
	return true
}

// provider is an interface for the k8s cluster providers
type provider interface {
	// setupK8sCluster sets up the provider specific cluster.
	// It returns true if it succeeded in its attempt.
	setupK8sCluster(t *testRunner) bool
	getKubeConfig() string
	teardownK8sCluster(t *testRunner)
	runGinkgoTestsInsideCluster(t *testRunner) bool
}

// local implements a provider to connect to an existing K8s cluster
type local struct {
	// kubeconfig is the kubeconfig of the cluster
	kubeconfig string
}

// newLocalProvider returns a new local provider
func newLocalProvider() *local {
	log.Println("🔧 Configuring tests to run on an existing cluster")
	return &local{}
}

// setupK8sCluster just validates the kubeconfig passed
// as the cluster is expected to be running already.
// It returns true if it succeeded in its attempt.
func (l *local) setupK8sCluster(*testRunner) bool {
	// Validate the kubeconfig
	if len(options.kubeconfig) > 0 {
		if validateKubeConfig(options.kubeconfig) {
			l.kubeconfig = options.kubeconfig
			return true
		}
	}

	log.Println("⚠️  Please pass a valid kubeconfig")
	return false
}

// getKubeConfig returns the Kubeconfig to connect to the cluster
func (l *local) getKubeConfig() string {
	return l.kubeconfig
}

// teardownK8sCluster is a no-op for local provider
// TODO: Maybe verify all the test resources are cleaned up here?
func (l *local) teardownK8sCluster(*testRunner) {}

// runGingoTestsInsideCluster is a no-op for local provider
func (l *local) runGinkgoTestsInsideCluster(*testRunner) bool {
	log.Fatal("❌ Running ginkgo tests inside cluster not supported for local provider.")
	return false
}

// kind implements a provider to control k8s clusters in KinD
type kind struct {
	// kubeconfig is the kubeconfig of the cluster
	kubeconfig string
	// cluster name
	clusterName string
	// kubernetes clientset
	clientset *kubernetes.Clientset
}

// newKindProvider returns a new kind provider
func newKindProvider() *kind {
	log.Println("🔧 Configuring tests to run on a KinD cluster")
	return &kind{}
}

// setupK8sCluster starts a K8s cluster using KinD
func (k *kind) setupK8sCluster(t *testRunner) bool {
	// Verify that docker is running
	if !t.execCommand([]string{"docker", "info"}, "docker info", true, false) {
		// Docker not running. Exit here as there is nothing to cleanup.
		log.Fatal("⚠️  Please ensure that docker daemon is running and accessible.")
	}
	log.Println("🐳 Docker daemon detected and accessible!")

	if !k.createKindCluster(t) {
		return false
	}

	// Load the operator docker image into cluster nodes
	if !k.loadImageToKindCluster("mysql/ndb-operator:latest", t) {
		return false
	}

	if options.inCluster {
		// Load e2e-tests docker image into cluster nodes
		if !k.loadImageToKindCluster("e2e-tests:latest", t) {
			return false
		}
	}
	return true
}

// createKindCluster creates a kind cluster 'ndb-e2e-test'
// It returns true on success.
func (k *kind) createKindCluster(t *testRunner) bool {
	// custom kubeconfig
	k.kubeconfig = filepath.Join(t.testDir, "_artifacts", ".kubeconfig")
	// kind cluster name
	k.clusterName = "ndb-e2e-test"
	// KinD k8s image used to run tests
	kindK8sNodeImage := kindK8sNodeImages[options.kindK8sVersion]
	// Build KinD command and args
	kindCreateCluster := append(kindCmd,
		// create cluster
		"create", "cluster",
		// cluster name
		"--name="+k.clusterName,
		// kubeconfig
		"--kubeconfig="+k.kubeconfig,
		// kind k8s node image to be used
		"--image="+kindK8sNodeImage,
		// cluster configuration
		"--config="+filepath.Join(t.testDir, "_config", "kind-3-node-cluster.yaml"),
	)

	// Run the command to create a cluster
	if !t.execCommand(kindCreateCluster, "kind create cluster", false, true) {
		log.Println("❌ Failed to create cluster using KinD")
		return false
	}
	log.Println("✅ Successfully started a KinD cluster")
	return true
}

// loadImageToKindCluster loads docker image to kind cluster
// It returns true on success.
func (k *kind) loadImageToKindCluster(image string, t *testRunner) bool {
	kindLoadImage := append(kindCmd,
		// load docker-image
		"load", "docker-image",
		// image name
		image,
		// cluster name
		"--name="+k.clusterName,
	)
	// Run the command to load docker image
	if !t.execCommand(kindLoadImage, "kind load docker-image", false, true) {
		log.Printf("❌ Failed to load '%s' image into KinD cluster", image)
		return false
	}
	log.Printf("✅ Successfully loaded '%s' image into the KinD cluster", image)
	return true
}

// getKubeConfig returns the Kubeconfig to connect to the cluster
func (k *kind) getKubeConfig() string {
	return k.kubeconfig
}

// getPodPhase returns a pod's phase in a given namespace
func (k *kind) getPodPhase(namespace string, podName string) (v1.PodPhase, error) {
	pod, err := k.clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return pod.Status.Phase, nil
}

// hasPodSucceeded checks if a given pod in a given namespace,
// has succeeded its execution.
// It returns true if pod has reached 'Succeeded' phase
func (k *kind) hasPodSucceeded(namespace string, podName string) bool {
	podPhase, err := k.getPodPhase(namespace, podName)
	if err != nil {
		log.Printf("❌ Error getting '%s' pod's phase: %s", podName, err)
		return false
	}

	if podPhase == v1.PodSucceeded {
		return true
	}
	return false
}

// isPodRunning checks if a given pod in a given namespace
// returns true, if pod is running
func (k *kind) isPodRunning(namespace string, podName string) (bool, error) {
	podPhase, err := k.getPodPhase(namespace, podName)
	if err != nil {
		return false, err
	}

	if podPhase == v1.PodRunning {
		return true, nil
	}
	// return false, nil by default,
	// indicating pod is in 'Pending' phase
	return false, nil
}

// runGinkgoTestInsideCluster runs all tests as a pod inside kind cluster
// It returns true if tests run successfully
func (k *kind) runGinkgoTestsInsideCluster(t *testRunner) bool {
	e2eArtifacts := filepath.Join(t.testDir, "_config", "k8s-deployment")
	// Build kubectl command to create kubernetes resources to run e2e-tests,
	// using e2e artifacts
	createE2eTestK8sResources := []string{
		"kubectl", "apply", "-f",
		// e2e-tests artifacts
		e2eArtifacts,
		// context that kubectl runs against
		"--context=kind-ndb-e2e-test",
		// kubeconfig
		"--kubeconfig=" + k.kubeconfig,
	}
	if !t.execCommand(createE2eTestK8sResources, "kubectl apply", false, true) {
		log.Println("❌ Failed to create kubernetes resources using e2e-test artifacts")
		return false
	}

	// create clientset to monitor pod phases
	k.clientset = buildClientsetFromConfig(k.kubeconfig)
	if k.clientset == nil {
		return false
	}

	// poll every second for 60 seconds to check if e2e-tests pod's are running
	err := wait.PollImmediate(1*time.Second, 60*time.Second,
		func() (done bool, err error) {
			return k.isPodRunning("default", "e2e-tests")
		})
	if err != nil {
		log.Printf("❌ Error running e2e-tests pod: %s", err)
		return false
	}

	// build kubectl command to print e2e-tests pod logs onto console
	e2eTestPodLogs := []string{
		"kubectl", "logs", "-f",
		// 'e2e-tests' pod
		"e2e-tests",
		// context that kubectl runs against
		"--context=kind-ndb-e2e-test",

		"--kubeconfig=" + k.kubeconfig,
	}
	if !t.execCommand(e2eTestPodLogs, "kubectl logs", false, true) {
		log.Println("❌ Failed to get e2e-tests pod logs.")
		return false
	}

	if !k.hasPodSucceeded("default", "e2e-tests") {
		log.Println("❌ There are test failures!")
		return false
	}
	log.Println("😊 All tests ran successfully!")
	return true
}

// teardownK8sCluster deletes the KinD cluster
func (k *kind) teardownK8sCluster(t *testRunner) {
	// Build KinD command and args
	kindCmdAndArgs := append(kindCmd,
		// create cluster
		"delete", "cluster",
		// cluster name
		"--name="+k.clusterName,
		// kubeconfig
		"--kubeconfig="+k.kubeconfig,
	)

	// Run the command
	t.execCommand(kindCmdAndArgs, "kind delete cluster", false, true)
}

// testRunner is the struct used to run the e2e test
type testRunner struct {
	// testDir is the absolute path of e2e test directory
	testDir string
	// p is the provider used to execute the test
	p provider
	// sigMutex is the mutex used to protect process
	// and ignoreSignals access across goroutines
	sigMutex sync.Mutex
	// process started by the execCommand
	// used to send signals when it is running
	// Access should be protected by sigMutex
	process *os.Process
	// passSignals enables passing signals to the
	// process started by the testRunner
	// Access should be protected by sigMutex
	passSignals bool
	// runDone is the channel used to signal that
	// the run method has completed. Used by
	// signalHandler to stop listening for signals
	runDone chan bool
}

// init sets up the testRunner
func (t *testRunner) init() {
	// Update log to print only line numbers
	log.SetFlags(log.Lshortfile)

	// Deduce test root directory
	_, currentFilePath, _, _ := runtime.Caller(0)
	t.testDir = filepath.Dir(currentFilePath)
}

func (t *testRunner) startSignalHandler() {
	// Create the runDone channel
	t.runDone = make(chan bool)
	// Start a go routine to handle signals
	go func() {
		// Create a channel to receive any signal
		sigs := make(chan os.Signal, 2)
		signal.Notify(sigs)

		// Handle all the signals as follows
		// - If a process is running and ignoreSignals
		//   is enabled, send the signal to the process.
		// - If a process is running and ignoreSignals
		//   is disabled, ignore the signal
		// - If no process is running, handle it appropriately
		// - Return when the main return signals done
		for {
			select {
			case sig := <-sigs:
				t.sigMutex.Lock()
				if t.process != nil {
					if t.passSignals {
						// Pass the signal to the process
						_ = t.process.Signal(sig)
					} // else it is ignored
				} else {
					// no process running - handle it
					if sig == syscall.SIGINT ||
						sig == syscall.SIGQUIT ||
						sig == syscall.SIGTSTP {
						// Test is being aborted
						// teardown the cluster and exit
						t.p.teardownK8sCluster(t)
						log.Fatalf("⚠️  Test was aborted!")
					}
				}
				t.sigMutex.Unlock()
			case <-t.runDone:
				// run method has completed - stop signal handler
				return
			}
		}
	}()
}

// stopSignalHandler stops the signal handler
func (t *testRunner) stopSignalHandler() {
	t.runDone <- true
}

// execCommand executes the command along with its arguments
// passed through commandAndArgs slice. commandName is a log
// friendly name of the command to be used in the logs. The
// command output can be suppressed by enabling the quiet
// parameter. passSignals should be set to true if the
// signals received by the testRunner needs be passed to the
// process started by this function.
// It returns true id command got executed successfully
// and false otherwise.
func (t *testRunner) execCommand(
	commandAndArgs []string, commandName string, quiet bool, passSignals bool) bool {
	// Create cmd struct
	cmd := exec.Command(commandAndArgs[0], commandAndArgs[1:]...)

	// Map the stout and stderr if not quiet
	if !quiet {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	// Protect cmd.Start() by sigMutex to avoid signal
	// handler wrongly processing the signals itself
	// after the process has been started
	t.sigMutex.Lock()

	// Start the command
	if err := cmd.Start(); err != nil {
		log.Printf("❌ Starting '%s' failed : %s", commandName, err)
		return false
	}

	// Setup variables to be used the signal handler
	// before unlocking the sigMutex
	t.process = cmd.Process
	t.passSignals = passSignals
	t.sigMutex.Unlock()
	defer func() {
		// Reset the signal handler variables before returning
		t.sigMutex.Lock()
		t.process = nil
		t.sigMutex.Unlock()
	}()

	// Wait for the process to complete
	if err := cmd.Wait(); err != nil {
		log.Printf("❌ Running '%s' failed : %s", commandName, err)
		return false
	}

	return true
}

// runGinkgoTests runs all the tests using ginkgo
// It returns true if all tests ran successfully
func (t *testRunner) runGinkgoTests() bool {
	// The ginkgo test command
	ginkgoTest := []string{
		"go", "run", "github.com/onsi/ginkgo/ginkgo",
		"-r",         // recursively run all suites in the given directory
		"-keepGoing", // keep running all test suites even if one fails
	}

	// Append the ginkgo directory to run the test on
	ginkgoTest = append(ginkgoTest, filepath.Join(t.testDir, "suites"))

	// Append arguments to pass to the testcases
	ginkgoTest = append(ginkgoTest, "--", "--kubeconfig="+t.p.getKubeConfig())

	// Execute it
	log.Println("🔨 Running tests using ginkgo : " + strings.Join(ginkgoTest, " "))
	if t.execCommand(ginkgoTest, "ginkgo", false, true) {
		log.Println("😊 All tests ran successfully!")
		return true
	}

	log.Println("❌ There are test failures!")
	return false
}

// run executes the complete e2e test
// It returns true if all tests run successfully
func (t *testRunner) run() bool {
	// Choose a provider
	var p provider
	if options.useKind {
		p = newKindProvider()
	} else {
		p = newLocalProvider()
	}
	// store it in testRunner
	t.p = p

	// Start signal handler
	t.startSignalHandler()

	// Setup the K8s cluster
	if !p.setupK8sCluster(t) {
		// Failed to setup cluster.
		// Cleanup resources and return.
		p.teardownK8sCluster(t)
		t.stopSignalHandler()
		return false
	}

	// testStatus is true when all tests run successfully, and
	// false if there are any test failures
	var testStatus bool
	// Run the tests
	if options.inCluster {
		// run tests as K8s pods inside kind cluster
		log.Printf("🔨 Running tests from inside the KinD cluster")
		testStatus = p.runGinkgoTestsInsideCluster(t)
	} else {
		// run tests as external go application
		testStatus = t.runGinkgoTests()
	}

	// Cleanup resources and return
	p.teardownK8sCluster(t)
	t.stopSignalHandler()

	return testStatus
}

func init() {

	flag.BoolVar(&options.useKind, "use-kind", false,
		"Use KinD to run the e2e tests.\nBy default, this is disabled and the tests will be run in an existing K8s cluster.")

	// use kubeconfig at $HOME/.kube/config as the default
	defaultKubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	flag.StringVar(&options.kubeconfig, "kubeconfig", defaultKubeconfig,
		"Kubeconfig of the existing K8s cluster to run tests on.\nThis will not be used if '--use-kind' is enabled.")

	flag.BoolVar(&options.inCluster, "in-cluster", false,
		"Run tests as K8s pod inside cluster.")

	// use v1.20 as default kind k8s version
	flag.StringVar(&options.kindK8sVersion, "kind-k8s-version", "1.20",
		"Kind k8s version used to run tests. Example usage: --kind-k8s-version=1.20")

}

// validatesCommandlineArgs validates if command line arguments,
// have been provided acceptable values.
// It exits program execution with status code 1 if validation fails.
// Currently, validates only --kind-k8s-version command line argument.
func validateCommandlineArgs() {
	_, exists := kindK8sNodeImages[options.kindK8sVersion]
	if !exists {
		var supportedKindK8sVersions string
		for key := range kindK8sNodeImages {
			supportedKindK8sVersions += ", " + key
		}
		log.Printf("❌ KinD version %s not supported. Supported KinD versions are%s", options.kindK8sVersion, supportedKindK8sVersions)
		os.Exit(1)
	}
}

func main() {
	flag.Parse()
	validateCommandlineArgs()
	t := testRunner{}
	t.init()
	if !t.run() {
		// exit with status code 1 on cluster setup failure or test failures
		os.Exit(1)
	}
}
