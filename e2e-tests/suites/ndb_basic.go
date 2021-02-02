package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/ginkgo"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"

	deployment_utils "github.com/mysql/ndb-operator/e2e-tests/utils/deployment"
	sfset_utils "github.com/mysql/ndb-operator/e2e-tests/utils/statefulset"
	yaml_utils "github.com/mysql/ndb-operator/e2e-tests/utils/yaml"
)

var (
	deploySuite = []string{
		"helm/crds/mysql.oracle.com_ndbs",
	}
)

var _ = ginkgo.SynchronizedBeforeSuite(func() []byte {
	setupSuite()
	return nil
},
	func([]byte) { doneChannelFunc() },
)

var _ = ginkgo.SynchronizedAfterSuite(func() { cleanupSuite() },
	func() { doneChannelFunc() }, 5000,
)

func doneChannelFunc() []byte { return nil }
func cleanupSuite() {
	klog.Infof("Deleting CRDs")
	yaml_utils.DeleteFromYamls("", deploySuite)
}

func setupSuite() {
	_, err := framework.LoadClientset()
	if err != nil {
		klog.Fatal("Error loading client: ", err)
	}

	klog.Infof("Creating CRDs")
	// at least atm all resources created as preparation are not tied
	yaml_utils.CreateFromYamls("", deploySuite)
}

var _ = framework.KubeDescribe("[Feature:ndb_basic]", func() {

	f := framework.NewDefaultFramework("ndb-basic")

	var ns string
	var c clientset.Interface
	var deploy = []string{
		"helm/templates/01-rbac",
		"artifacts/deployment/ndb-operator-rbac",
	}

	ginkgo.BeforeEach(func() {
		ns = f.Namespace.Name
		c = f.ClientSet
		ginkgo.By(fmt.Sprintf("Running in namespace %s creating resources", ns))

		ginkgo.By(fmt.Sprintf("Create RBACS"))
		yaml_utils.CreateFromYamls(ns, deploy)

		ginkgo.By(fmt.Sprintf("Create operator deployment"))
		yaml_utils.CreateFromYaml(ns, "artifacts/deployment", "ndb-operator")

		err := deployment_utils.WaitForDeploymentComplete(c, ns, "ndb-operator", 2*time.Second, 5*time.Minute)
		framework.ExpectNoError(err)
	})

	ginkgo.AfterEach(func() {

		ginkgo.By("Cleaning up after each")

		yaml_utils.DeleteFromYaml(ns, "", "artifacts/deployment/ndb-operator")

		err := e2epod.WaitForPodToDisappear(c, "ndb-operator", ns, labels.Everything(), time.Second, wait.ForeverTestTimeout)
		framework.ExpectNoError(err)

		// crds and our rbacs are not child of namespace
		ginkgo.By(fmt.Sprintf("Deleting from yamls from namespace %s", ns))
		yaml_utils.DeleteFromYamls(ns, deploy)
	})

	framework.KubeDescribe("[Feature:basic creation and teardown]", func() {
		ginkgo.It("Create and delete a basic cluster", func() {
			ginkgo.By(fmt.Sprintf("Creating ndb resource to test the example file"))
			yaml_utils.CreateFromYaml(ns, "artifacts/examples", "example-ndb")

			err := sfset_utils.WaitForStatefulSetComplete(c, ns, "example-ndb-ndbd", 2*time.Second, 5*time.Minute)
			framework.ExpectNoError(err)

			err = sfset_utils.WaitForStatefulSetComplete(c, ns, "example-ndb-mgmd", 2*time.Second, 5*time.Minute)
			framework.ExpectNoError(err)

			err = deployment_utils.WaitForDeploymentComplete(c, ns, "example-ndb-mysqld", 2*time.Second, 5*time.Minute)
			framework.ExpectNoError(err)

			ginkgo.By(fmt.Sprintf("Deleting ndb resource after creation"))

			yaml_utils.DeleteFromYaml(ns, "artifacts/examples", "example-ndb")

			err = sfset_utils.WaitForStatefulSetToDisappear(c, ns, "example-ndb-ndbd", 2*time.Second, 5*time.Minute)
			framework.ExpectNoError(err)

			err = sfset_utils.WaitForStatefulSetToDisappear(c, ns, "example-ndb-mgmd", 2*time.Second, 5*time.Minute)
			framework.ExpectNoError(err)

			err = deployment_utils.WaitForDeploymentToDisappear(c, ns, "example-ndb-mysqld", 2*time.Second, 5*time.Minute)
			framework.ExpectNoError(err)
		})
	})

	framework.KubeDescribe("Creating and deleting a simple pod", func() {
		ginkgo.It("busybox basic", func() {

			klog.Infof("IT: busybox basic")

			justAnExample := yaml_utils.YamlFile("e2e-tests/_manifests", "busybox")
			var err error
			justAnExample, err = yaml_utils.ReplaceAllProperties(justAnExample, "namespace", ns)

			if err != nil {
				klog.Fatalf("Error parsing %s\n", justAnExample)
			}

			framework.RunKubectlOrDieInput(ns, justAnExample, "create", "-f", "-")

			// created in default by this particular file
			err = e2epod.WaitForPodNameRunningInNamespace(c, "busybox", ns)
			framework.ExpectNoError(err)

			// PodClient() is only for framework namespace
			//err = f.PodClient().Delete(context.TODO(), "busybox", *v1.NewDeleteOptions(30))
			err = c.CoreV1().Pods(ns).Delete(context.TODO(), "busybox", *metav1.NewDeleteOptions(30))
			framework.ExpectNoError(err)

			err = e2epod.WaitForPodToDisappear(c, "busybox", ns, labels.Everything(), time.Second, wait.ForeverTestTimeout)
			framework.ExpectNoError(err)

		})
	})
})
