package e2e

import (
	"context"
	"time"

	"github.com/onsi/ginkgo"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
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
func cleanupSuite()           {}

func setupSuite() {
	_, err := framework.LoadClientset()
	if err != nil {
		klog.Fatal("Error loading client: ", err)
	}
}

var _ = framework.KubeDescribe("[Feature:ndb_basic]", func() {

	f := framework.NewDefaultFramework("ndb-basic")

	var ns string
	var c clientset.Interface
	ginkgo.BeforeEach(func() {
		ns = f.Namespace.Name
		c = f.ClientSet
		klog.Infof("Running in namespace %s", ns)
	})

	framework.KubeDescribe("Just do nothing", func() {
		ginkgo.It("should create environment here", func() {
		})
	})

	framework.KubeDescribe("Creating and deleting a simple pod", func() {
		ginkgo.It("should create environment", func() {

			justAnExample := YamlFile("artifacts/examples", "busybox")
			var err error
			justAnExample, err = ReplaceAllProperties(justAnExample, "namespace", ns)

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
