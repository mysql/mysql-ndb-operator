// Copyright (c) 2020, 2024, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// Tool to run end-to-end tests using Ginkgo and Kind/Minikube

//go:build ignore
// +build ignore

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
	"syscall"

	podutils "github.com/mysql/ndb-operator/e2e-tests/utils/pods"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// logDrawLine draws a dashed line using log
func logDrawLine() {
	// Temporarily disable flags in log
	logFlags := log.Flags()
	log.SetFlags(0)
	// Draw a dashed line
	log.Println("\n------------------------------")
	// Restore log flags
	log.SetFlags(logFlags)
}

// Command line options
var options struct {
	useKind         bool
	kubeconfig      string
	runOutOfCluster bool
	kindK8sVersion  string
	suites          string
	ginkgoFocusFile string
	verbose         bool
	// NDB Operator image to be used by the tests
	ndbOperatorImage string
}

// K8s image used by KinD to bring up cluster
// https://github.com/kubernetes-sigs/kind/releases
var kindK8sNodeImages = map[string]string{
	"1.19": "kindest/node:v1.19.16@sha256:476cb3269232888437b61deca013832fee41f9f074f9bed79f57e4280f7c48b7",
	"1.20": "kindest/node:v1.20.15@sha256:a32bf55309294120616886b5338f95dd98a2f7231519c7dedcec32ba29699394",
	"1.21": "kindest/node:v1.21.14@sha256:9d9eb5fb26b4fbc0c6d95fa8c790414f9750dd583f5d7cee45d92e8c26670aa1",
	"1.22": "kindest/node:v1.22.15@sha256:7d9708c4b0873f0fe2e171e2b1b7f45ae89482617778c1c875f1053d4cef2e41",
	"1.23": "kindest/node:v1.23.12@sha256:ef453bb7c79f0e3caba88d2067d4196f427794086a7d0df8df4f019d5e336b61",
	"1.24": "kindest/node:v1.24.6@sha256:577c630ce8e509131eab1aea12c022190978dd2f745aac5eb1fe65c0807eb315",
	"1.25": "kindest/node:v1.25.2@sha256:f52781bc0d7a19fb6c405c2af83abfeb311f130707a0e219175677e366cc45d1",
}

var (
	kindCmd = []string{"systemd-run", "--scope", "--user", "-p", "Delegate=yes", "go", "run", "sigs.k8s.io/kind"}
)

// shortUUID returns a short UUID to be used by the methods
func shortUUID() string {
	return string(uuid.NewUUID())[:8]
}

// provider is an interface for the k8s cluster providers
type provider interface {
	// setupK8sCluster sets up the provider specific cluster.
	// It returns true if it succeeded in its attempt.
	setupK8sCluster(t *testRunner) bool
	loadImageIntoK8sCluster(t *testRunner, image string) bool
	getKubeConfig() string
	getKubectlCommand() []string
	getClientset() kubernetes.Interface
	teardownK8sCluster(t *testRunner)
	dumpLogs(t *testRunner)
}

// providerDefaults defines the common fields and
// methods to be used by the other providers
type providerDefaults struct {
	// kubeconfig is the kubeconfig of the cluster
	kubeconfig string
	// kubernetes clientset
	clientset kubernetes.Interface
}

// getKubeConfig returns the Kubeconfig to connect to the cluster
func (p *providerDefaults) getKubeConfig() string {
	return p.kubeconfig
}

// getKubectlCommand returns the kubectl command with
// the required arguments to connect to the K8s cluster.
func (p *providerDefaults) getKubectlCommand() []string {
	return []string{
		"kubectl",
		"--kubeconfig=" + p.kubeconfig,
	}
}

// getClientset returns the clientset that can be used
// to connect to the K8s Cluster managed by the provider.
func (p *providerDefaults) getClientset() kubernetes.Interface {
	return p.clientset
}

// initClientsetFromKubeconfig creates a kubernetes clientset from the given kubeconfig
func (p *providerDefaults) initClientsetFromKubeconfig() (success bool) {
	config, err := clientcmd.BuildConfigFromFlags("", p.kubeconfig)
	if err != nil {
		log.Printf("❌ Error building config from kubeconfig '%s': %s", p.kubeconfig, err)
		return false
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Printf("❌ Error creating new kubernetes clientset: %s", err)
		return false
	}

	// success
	p.clientset = clientset
	return true
}

// loadImageIntoK8sCluster implements a default no-op method for other providers
func (p *providerDefaults) loadImageIntoK8sCluster(*testRunner, string) bool {
	return true
}

// local implements a provider to connect to an existing K8s cluster
type local struct {
	providerDefaults
}

// newLocalProvider returns a new local provider
func newLocalProvider() *local {
	log.Println("🔧 Configuring tests to run on an existing cluster")
	return &local{}
}

// setupK8sCluster just connects to the Kubernetes Server using the
// kubeconfig passed as the cluster is expected to be running already.
func (l *local) setupK8sCluster(*testRunner) (success bool) {
	// Connect to the K8s Cluster using the kubeconfig
	if len(options.kubeconfig) > 0 {
		l.kubeconfig = options.kubeconfig
		if !l.initClientsetFromKubeconfig() {
			return false
		}

		// Retrieve version and verify
		var k8sVersion *version.Info
		var err error
		if k8sVersion, err = l.clientset.Discovery().ServerVersion(); err != nil {
			log.Printf("❌ Error finding out the version of the K8s cluster : %s", err)
			return false
		}

		log.Printf("👍 Successfully validated kubeconfig and connected to the Kubernetes Server.\n"+
			"Kubernetes Server version : %s", k8sVersion.String())
		return true
	}

	log.Println("⚠️  Please pass a valid kubeconfig")
	return false
}

// teardownK8sCluster is a no-op for local provider
// TODO: Maybe verify all the test resources are cleaned up here?
func (l *local) teardownK8sCluster(*testRunner) {}

// dumpLogs is a no-op for local provider
func (l *local) dumpLogs(*testRunner) {}

// kind implements a provider to control k8s clusters in KinD
type kind struct {
	providerDefaults
	// cluster name
	clusterName string
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

	// init clientset to the k8s cluster
	if !k.initClientsetFromKubeconfig() {
		return false
	}

	return true
}

// createKindCluster creates a kind cluster 'ndb-e2e-test'
// It returns true on success.
func (k *kind) createKindCluster(t *testRunner) bool {
	// custom kubeconfig
	k.kubeconfig = filepath.Join(t.testDir, "_artifacts", ".kubeconfig")
	// kind cluster name - append an uuid to allow
	// running multiple tests in parallel
	k.clusterName = "ndb-e2e-test-" + shortUUID()
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

// loadImageIntoK8sCluster loads docker image to kind cluster
func (k *kind) loadImageIntoK8sCluster(t *testRunner, image string) (success bool) {
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

// getKubectlCommand returns the kubectl command with
// the required arguments to connect to the K8s cluster.
func (k *kind) getKubectlCommand() []string {
	return append(
		k.providerDefaults.getKubectlCommand(),
		// context that kubectl runs against
		"--context=kind-"+k.clusterName,
	)
}

// dumpLogs exports the KinD logs to _artifacts/kind-logs
func (k *kind) dumpLogs(t *testRunner) {
	// Build KinD command and args
	kindCmdAndArgs := append(kindCmd,
		// kind export logs
		"export", "logs",
		"--name=ndb-e2e-test",
		"./e2e-tests/_artifacts/kind-logs",
	)

	t.execCommand(kindCmdAndArgs, "kind export logs", false, true)
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
	// ctx and the related cancel function
	// to be used across this tool.
	ctx    context.Context
	cancel context.CancelFunc
	// testDir is the absolute path of e2e test directory
	testDir string
	// p is the provider used to execute the test
	p provider
	// e2eTestImageName is the name of the e2e
	// tests docker image
	e2eTestImageName string
	// pod name and namespace
	e2eTestPodName, e2eTestPodNamespace string
	// cleanup methods that need to be executed after
	// the test completes. The methods in the slice
	// are run in reverse order.
	cleanupMethods []func()
	// Boolean flag that indicates if the test was aborted
	aborted bool
}

// init sets up the testRunner
func (t *testRunner) init(ctx context.Context) {
	// Update log to print only line numbers
	log.SetFlags(log.Lshortfile)

	// Deduce test root directory
	var _, currentFilePath, _, _ = runtime.Caller(0)
	t.testDir = filepath.Dir(currentFilePath)

	// By default, use the latest e2e-tests image
	t.e2eTestImageName = "localhost/e2e-tests"

	// Run the pod in the default namespace
	t.e2eTestPodNamespace = "default"
	t.e2eTestPodName = "e2e-tests-pod"

	// Create a context with cancel
	// and store it in testRunner
	t.ctx, t.cancel = context.WithCancel(ctx)
	t.deferCleanup(func() {
		t.cancel()
	})
}

func (t *testRunner) startSignalHandler() {
	// Start a go routine to handle signals
	go func() {
		// Create a channel to receive interrupt and kill signals
		sigs := make(chan os.Signal, 2)
		signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

		for {
			select {
			case <-sigs:
				t.aborted = true
				if !options.runOutOfCluster && t.p != nil && t.p.getClientset() != nil {
					// The test is being aborted and test was to be run
					// inside a pod. Delete the pod if it is running already.
					_ = t.deleteE2eTestsPodIfExists()
				}
				// Cancel the current context and replace it with a
				// new context for the rest of the execution. Any methods
				// or commands running after this cancel (mostly test
				// cleanup methods and commands) will use this new context.
				// This also means that if required, the cleanup methods
				// can again be interrupted by sending in a ctrl+c.
				currentCtxCancel := t.cancel
				t.ctx, t.cancel = context.WithCancel(context.Background())
				currentCtxCancel()
			case <-t.ctx.Done():
				// Context is done => stopSignalHandler has been called.
				// Note : The cancel called in the previous case will never
				// trigger this case as the previous case replaces the t.ctx
				// before calling cancel.
				return
			}
		}
	}()
}

// stopSignalHandler stops the signal handler by cancelling the t.ctx
func (t *testRunner) stopSignalHandler() {
	t.cancel()
}

// deferCleanup adds the given method to the cleanupMethods slice
func (t *testRunner) deferCleanup(cleanupMethod func()) {
	t.cleanupMethods = append(t.cleanupMethods, cleanupMethod)
}

// cleanup runs all the cleanupMethods in reverse order
func (t *testRunner) cleanup() {
	if t.cleanupMethods == nil {
		// nothing to do
		return
	}
	log.Println("🧹 Cleaning up test resources..")
	// Run the cleanup methods in reverse order
	for i := len(t.cleanupMethods) - 1; i >= 0; i-- {
		t.cleanupMethods[i]()
	}
}

// execCommand executes the command along with its arguments
// passed through commandAndArgs slice. commandName is a log
// friendly name of the command to be used in the logs. The
// command output can be suppressed by enabling the quiet
// parameter. killOnInterrupt should be set to true if the
// command being run should be killed if this tool receives
// an interrupt signal.
// It returns true if the command got executed successfully
// without any interruption and false otherwise.
func (t *testRunner) execCommand(
	commandAndArgs []string, commandName string, quiet bool, killOnInterrupt bool) (cmdSucceeded bool) {
	// Create cmd struct
	var cmd *exec.Cmd
	if killOnInterrupt {
		// Create the command with t.ctx so that when ctrl+c is
		// called, the command can be killed by cancelling the context.
		cmd = exec.CommandContext(t.ctx, commandAndArgs[0], commandAndArgs[1:]...)
	} else {
		cmd = exec.Command(commandAndArgs[0], commandAndArgs[1:]...)
		// Disable the ctrl+c from passing to the command by
		// requesting it to start in its own process group
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
		}
	}

	// Map the stout and stderr if not quiet
	if !quiet {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	// Enable DOCKER_BUILDKIT for docker commands
	cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")

	// Run the command
	if err := cmd.Run(); err != nil {
		log.Printf("❌ Running '%s' failed : %s", commandName, err)
		return false
	}

	return true
}

// getGinkgoTestCommand builds and returns the ginkgo test command
// that will be executed by the testRunner.
func (t *testRunner) getGinkgoTestCommand(suiteDir string, testcaseOptions ...string) []string {
	// The ginkgo test command
	ginkgoTestCmd := []string{
		"go", "run", "github.com/onsi/ginkgo/v2/ginkgo",
		"-r",          // recursively run all suites in the given directory
		"-keep-going", // keep running all test suites even if one fails
		"-timeout=3h", // increase total Ginkgo run timeout
	}

	if options.verbose {
		// Print extra logs
		ginkgoTestCmd = append(ginkgoTestCmd, "-v")
	}

	if options.ginkgoFocusFile != "" {
		ginkgoTestCmd = append(ginkgoTestCmd, "--focus-file="+options.ginkgoFocusFile)
	}

	if options.suites == "" {
		// Run all test suites
		ginkgoTestCmd = append(ginkgoTestCmd, suiteDir)
	} else {
		// Append ginkgo test suite directories, to run specific testsuites
		for _, suite := range strings.Split(options.suites, ",") {
			ginkgoTestCmd = append(ginkgoTestCmd,
				filepath.Join(suiteDir, suite))
		}
	}

	// End of options to be passed to Ginkgo
	// Handle options to be passed directly to testcase
	if options.ndbOperatorImage != "" {
		testcaseOptions = append(testcaseOptions,
			"--ndb-operator-image="+options.ndbOperatorImage)
	}

	// Append any testcase options to ginkgoTestCmd
	if testcaseOptions != nil {
		ginkgoTestCmd = append(ginkgoTestCmd, "--")
		ginkgoTestCmd = append(ginkgoTestCmd, testcaseOptions...)
	}

	return ginkgoTestCmd
}

// runGinkgoTests runs all the tests using ginkgo
// It returns true if all tests ran successfully
func (t *testRunner) runGinkgoTests() bool {
	log.Println("🔨 Running tests from outside the K8s cluster")
	suiteDir := filepath.Join(t.testDir, "suites")
	// get ginkgo command to run specific test suites
	ginkgoTestCmd := t.getGinkgoTestCommand(suiteDir, "--kubeconfig="+t.p.getKubeConfig())

	// Execute it
	log.Println("🔨 Running tests using ginkgo : " + strings.Join(ginkgoTestCmd, " "))
	cmdSucceeded := t.execCommand(ginkgoTestCmd, "ginkgo", false, true)
	// Draw a dashed line to mark the end of logs
	logDrawLine()

	if cmdSucceeded {
		log.Println("😊 All tests ran successfully!")
		return true
	} else if t.aborted {
		// The test was aborted.
		log.Println("⚠️  Test was aborted!")
		return false
	}

	// Or else, the test failed
	log.Println("❌ There are test failures!")
	return false
}

// buildE2ETestImage builds the e2e test docker image
func (t *testRunner) buildE2ETestImage() bool {
	// Build the docker command
	// docker build -t e2e-tests -f docker/e2e-tests/Dockerfile .
	dockerBuildCmd := []string{
		"docker", "build",
		"-t", t.e2eTestImageName,
		"-f", filepath.Join(t.testDir, "..", "docker", "e2e-tests", "Dockerfile"),
	}

	// Append any proxies set in env to the docker command
	if httpProxy, exists := os.LookupEnv("http_proxy"); exists {
		dockerBuildCmd = append(dockerBuildCmd,
			"--build-arg", "http_proxy="+httpProxy,
		)
	}
	if httpsProxy, exists := os.LookupEnv("https_proxy"); exists {
		dockerBuildCmd = append(dockerBuildCmd,
			"--build-arg", "https_proxy="+httpsProxy,
		)
	}

	// Append docker context
	dockerBuildCmd = append(dockerBuildCmd, filepath.Join(t.testDir, ".."))

	// Run the docker build command to build the e2e tests image
	if !t.execCommand(dockerBuildCmd, "docker build e2e-tests", false, true) {
		log.Println("❌ Error building the e2e-tests docker image")
		return false
	}

	// setup defer to delete the image during cleanup
	t.deferCleanup(func() {
		t.removeE2ETestImage()
	})

	return true
}

// removeE2ETestImage removes the e2e test image built by buildE2ETestImage
func (t *testRunner) removeE2ETestImage() {
	dockerRemoveCmd := []string{
		"docker", "rmi", t.e2eTestImageName,
	}
	if !t.execCommand(dockerRemoveCmd, "docker rmi e2e-tests", true, true) {
		// Just print the warning and ignore the error
		log.Println("⚠️  Error removing the e2e-tests docker image")
	}
}

// newE2ETestPod defines and returns a new e2e-tests pod that runs the given command
func (t *testRunner) newE2ETestPod(command []string) *v1.Pod {
	// e2e-tests container to be run inside the pod
	e2eTestsContainer := v1.Container{
		// Container name, image used and image pull policy
		Name:            "e2e-tests-container",
		Image:           t.e2eTestImageName,
		ImagePullPolicy: v1.PullNever,
		// The e2e-tests-image has an entrypoint script that will
		// run the tests and handle signals. Add the command to
		// be run as container Args.
		Args: command,
	}

	e2eTestsPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			// Pod name and namespace
			Name:      t.e2eTestPodName,
			Namespace: t.e2eTestPodNamespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{e2eTestsContainer},
			// Service account with necessary RBAC authorization
			ServiceAccountName: "e2e-tests-service-account",
			RestartPolicy:      v1.RestartPolicyNever,
		},
	}

	return e2eTestsPod
}

// deleteE2eTestsPodIfExists deletes the e2e test pod if it is running
func (t *testRunner) deleteE2eTestsPodIfExists() (err error) {
	if err = podutils.DeletePodIfExists(
		t.ctx, t.p.getClientset(), t.e2eTestPodNamespace, t.e2eTestPodName); err != nil {
		log.Printf("❌ Failed to delete the e2e test pod : %s", err.Error())
	}
	return err
}

// dumpPodInfo dumps the pod details using kubectl describe
func (t *testRunner) dumpPodInfo(namespace, name string) {
	kubectlCmd := append(
		t.p.getKubectlCommand(),
		"--namespace="+namespace,
		"describe", "pods", name)

	// ignore command exit value
	t.execCommand(kubectlCmd, "kubectl describe", false, true)
}

// waitForPodToStart waits until all the containers in the given pod are started
func (t *testRunner) waitForPodToStart(namespace, name string) (podStarted bool) {
	if err := podutils.WaitForPodToStart(t.ctx, t.p.getClientset(), namespace, name); err != nil {
		log.Printf("❌ Error waiting for the e2e-tests pod to start : %s", err)
		return false
	}

	return true
}

// waitForPodToTerminate waits until all the containers in the given pod are terminated
func (t *testRunner) waitForPodToTerminate(namespace, name string) (podTerminated bool) {
	if err := podutils.WaitForPodToTerminate(t.ctx, t.p.getClientset(), namespace, name); err != nil {
		log.Printf("❌ Error waiting for the e2e-tests pod to terminate : %s", err)
		return false
	}

	return true
}

// runGinkgoTestsInsideK8sCluster runs the tests as a pod in the K8s Cluster
func (t *testRunner) runGinkgoTestsInsideK8sCluster() (success bool) {
	log.Println("🔨 Running tests from inside the K8s cluster")

	// Get the kubectl command to be used for creating
	// and monitoring the test and related resources.
	kubectlCommand := t.p.getKubectlCommand()

	// Create the required RBACs in K8s Cluster
	e2eArtifacts := filepath.Join(t.testDir, "_config", "k8s-deployment")
	createE2eTestK8sResources := append(kubectlCommand,
		"apply", "-f",
		// e2e-tests artifacts
		e2eArtifacts,
	)
	if !t.execCommand(createE2eTestK8sResources, "kubectl apply", false, true) {
		log.Println("❌ Failed to create the required RBACs for the e2e tests pod")
		return false
	}
	log.Println("✅ Successfully created the required RBACs for the e2e tests pod")

	// Setup defer to cleanup RBACs before returning
	t.deferCleanup(func() {
		deleteE2eTestK8sResources := append(kubectlCommand,
			"delete", "-f", e2eArtifacts,
		)
		if !t.execCommand(deleteE2eTestK8sResources, "kubectl delete", true, false) {
			log.Println("❌ Failed to cleanup the RBACs created for the e2e tests pod")
		}
	})

	// Get the ginkgo command to be run.
	// By default, all suites under 'e2e-tests/suites' will be run.
	// If '-suites' flag is passed, the mentioned suites will be run.
	ginkgoTestCmd := t.getGinkgoTestCommand("e2e-tests/suites")

	// Create a pod that runs the tests using the above command
	clientset := t.p.getClientset()
	e2eTestPod := t.newE2ETestPod(ginkgoTestCmd)
	_, err := clientset.CoreV1().Pods(e2eTestPod.Namespace).Create(t.ctx, e2eTestPod, metav1.CreateOptions{})
	if err != nil {
		log.Printf("❌ Error creating e2e-test pod: %s", err)
		return false
	}
	log.Println("✅ Successfully created the the e2e test pod")

	// Setup defer to clean up pod after completion
	t.deferCleanup(func() {
		// delete the e2e test pod
		_ = t.deleteE2eTestsPodIfExists()
	})

	// Wait for the pods to start
	if !t.waitForPodToStart(e2eTestPod.Namespace, e2eTestPod.Name) {
		return false
	}
	log.Println("🏃 The e2e tests pod has started running")

	// Redirect pod logs to console
	log.Println("📃 Redirecting pod logs to console...")
	// Run 'kubectl logs -f e2e-test-pod'
	e2eTestPodLogs := append(
		kubectlCommand,
		"logs", "-f",
		// e2e tests pod
		"--namespace="+e2eTestPod.Namespace,
		e2eTestPod.Name,
	)
	if !t.execCommand(e2eTestPodLogs, "kubectl logs", false, false) {
		log.Println("❌ Failed to get e2e-tests pod logs.")
		return false
	}

	// Draw a dashed line to mark the end of logs
	logDrawLine()

	if t.aborted {
		// The test was aborted.
		// No need to check pod status.
		log.Println("⚠️  Test was aborted!")
		return false
	}

	// Wait for the pod to terminate
	if !t.waitForPodToTerminate(e2eTestPod.Namespace, e2eTestPod.Name) {
		return false
	}

	if !podutils.HasPodSucceeded(t.ctx, t.p.getClientset(), e2eTestPod.Namespace, e2eTestPod.Name) {
		log.Println("❌ There are test failures!")
		return false
	}
	log.Println("😊 All tests ran successfully!")
	return true
}

func (t *testRunner) loadImagesIntoK8sCluster() bool {
	// Some providers require manual loading of images into their nodes.
	// Load the operator docker image in the K8s Cluster nodes
	if options.ndbOperatorImage == "" {
		operatorVersionFile := filepath.Join(t.testDir, "..", "VERSION")
		versionBytes, err := os.ReadFile(operatorVersionFile)
		if err != nil {
			log.Printf("❌ Failed to read NDB Operator version : %s", err)
			return false
		}
		versionNumber := string(versionBytes)
		// trim the newline from version
		versionNumber = versionNumber[:len(versionNumber)-1]
		options.ndbOperatorImage = "localhost/mysql/ndb-operator:" + versionNumber
	}
	if !t.p.loadImageIntoK8sCluster(t, options.ndbOperatorImage) {
		return false
	}

	if !options.runOutOfCluster {
		// Load e2e-tests docker image into cluster nodes for in-cluster runs
		if !t.p.loadImageIntoK8sCluster(t, t.e2eTestImageName) {
			return false
		}
	}

	// Load required MySQL Cluster images
	// This is done here to avoid delays inside the actual testcases.
	for _, image := range []string{
		// default image used by the tests
		"container-registry.oracle.com/mysql/community-cluster:latest",
		// images used by the upgrade_test.go testcase
		"container-registry.oracle.com/mysql/community-cluster:8.0.28",
		"container-registry.oracle.com/mysql/community-cluster:8.0.30",
	} {
		// Pull the image into local docker
		dockerPullCmd := []string{
			"docker", "pull", image,
		}
		if !t.execCommand(dockerPullCmd, "docker pull "+image, true, true) {
			log.Printf("❌ Failed to pull docker image %q", image)
			return false
		}

		// Load the image into K8s worker nodes
		if !t.p.loadImageIntoK8sCluster(t, image) {
			return false
		}
	}

	return true
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

	// testRunner should cleanup resources before returning
	defer t.cleanup()

	// Start signal handler
	t.startSignalHandler()
	// setup defer to clear signal handler
	t.deferCleanup(func() {
		t.stopSignalHandler()
	})

	// setup defer to teardown cluster if
	// the method returns after this point
	t.deferCleanup(func() {
		t.p.teardownK8sCluster(t)
	})

	// Build the test image if running inside KinD Cluster
	if options.useKind && !options.runOutOfCluster {
		// Append a short uuid to the image tag to allow
		// multiple run-e2e-tests to run in parallel.
		t.e2eTestImageName += ":" + shortUUID()

		if !t.buildE2ETestImage() {
			return false
		}
	}

	// Set up the K8s cluster
	if !p.setupK8sCluster(t) {
		// Failed to set up cluster.
		return false
	}

	// Load required images into K8s Cluster
	if !t.loadImagesIntoK8sCluster() {
		// Failed to load required images in K8s Cluster
		return false
	}

	// Run the tests
	if options.runOutOfCluster {
		// run tests as external go application
		return t.runGinkgoTests()
	} else {
		// run tests as K8s pods inside kind cluster
		return t.runGinkgoTestsInsideK8sCluster()
	}
}

func init() {

	flag.BoolVar(&options.useKind, "use-kind", false,
		"Use KinD to run the e2e tests.\n"+
			"By default, this is disabled and the tests will be run in an existing K8s cluster.")

	// use kubeconfig at $HOME/.kube/config as the default
	defaultKubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	flag.StringVar(&options.kubeconfig, "kubeconfig", defaultKubeconfig,
		"Kubeconfig of the existing K8s cluster to run tests on.\n"+
			"This will not be used if '--use-kind' is enabled.")

	flag.BoolVar(&options.runOutOfCluster, "run-out-of-cluster", false,
		"Enable this to run tests from outside the K8s cluster.\n"+
			"By default, this is not enabled and the tests will be run as a pod from inside K8s Cluster.")

	// use v1.23 as default kind k8s version
	flag.StringVar(&options.kindK8sVersion, "kind-k8s-version", "1.23",
		"Kind k8s version used to run tests.\n"+
			"Example usage: -kind-k8s-version=1.23")

	// test suites to be run.
	flag.StringVar(&options.suites, "suites", "",
		"Test suites that needs to be run.\n"+
			"Example usage: -suites=mysql,basic")

	flag.StringVar(&options.ginkgoFocusFile, "ginkgo.focus-file", "",
		"Value to be passed to the ginkgo's focus-file flag.\n"+
			"Use this to filter specs to run based on their location in files.\n"+
			"More details : https://onsi.github.io/ginkgo/#location-based-filtering")

	// Add verbose for extra logs.
	flag.BoolVar(&options.verbose, "v", false,
		"Enable verbose mode in ginkgo. By default this is disabled.")

	flag.StringVar(&options.ndbOperatorImage,
		"ndb-operator-image", "",
		"The NDB Operator image to be used by the e2e tests.\n"+
			"By default, the image specified in the helm chart will be used.")
}

// validatesCommandlineArgs validates if command line arguments,
// have been provided acceptable values.
// It exits program execution with status code 1 if validation fails.
func validateCommandlineArgs() {
	// validate supported kind K8s version
	_, exists := kindK8sNodeImages[options.kindK8sVersion]
	if !exists {
		var supportedKindK8sVersions string
		for key := range kindK8sNodeImages {
			supportedKindK8sVersions += ", " + key
		}
		log.Printf("❌ KinD version %s not supported. Supported KinD versions are%s", options.kindK8sVersion, supportedKindK8sVersions)
		os.Exit(1)
	}

	// validate if test suites exist
	// Deduce test root directory
	var _, currentFilePath, _, _ = runtime.Caller(0)
	testDir := filepath.Dir(currentFilePath)
	suitesDir := filepath.Join(testDir, "suites")
	for _, suite := range strings.Split(options.suites, ",") {
		suite = filepath.Join(suitesDir, suite)
		if _, err := os.Stat(suite); os.IsNotExist(err) {
			log.Printf("❌ Test suite %s doesn't exist.", suite)
			log.Printf("Please find available test suites in %s.", suitesDir)
			os.Exit(1)
		}
	}
}

func main() {
	flag.Parse()
	validateCommandlineArgs()
	t := testRunner{}
	t.init(context.Background())
	if !t.run() {
		// exit with status code 1 on cluster setup failure or test failures
		os.Exit(1)
	}
}
