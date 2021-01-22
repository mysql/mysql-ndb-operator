package e2e

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/onsi/ginkgo"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	"k8s.io/kubernetes/test/e2e/framework/testfiles"
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

var _ = framework.KubeDescribe("[Feature:Example]", func() {

	f := framework.NewDefaultFramework("examples")

	var ns string
	var c clientset.Interface
	ginkgo.BeforeEach(func() {
		ns = f.Namespace.Name
		c = f.ClientSet
		klog.Infof("Running in namespace %s", ns)
	})

	framework.KubeDescribe("Creating something", func() {
		ginkgo.It("should create a namespace", func() {

			labels := make(map[string]string)
			nns, err := f.CreateNamespace("ndb-4-test", labels)
			framework.ExpectNoError(err)

			klog.Infof("Created namespace: %s", nns.Name)

			ginkgo.By("checking if name and namespace were passed correctly")
		})
	})

	framework.KubeDescribe("Creating and deleting a simple pod", func() {
		ginkgo.It("should create environment", func() {

			justAnExample := yamlFile("artifacts/examples", "busybox")

			framework.RunKubectlOrDieInput(ns, justAnExample, "create", "-f", "-")

			// created in default by this particular file
			err := e2epod.WaitForPodNameRunningInNamespace(c, "busybox", "default")
			framework.ExpectNoError(err)

			// PodClient() is only for framework namespace
			//err = f.PodClient().Delete(context.TODO(), "busybox", *v1.NewDeleteOptions(30))
			err = c.CoreV1().Pods("default").Delete(context.TODO(), "busybox", *metav1.NewDeleteOptions(30))
			framework.ExpectNoError(err)

			err = e2epod.WaitForPodToDisappear(c, "busybox", "default", labels.Everything(), time.Second, wait.ForeverTestTimeout)
			framework.ExpectNoError(err)

		})
	})
})

func yamlFile(test, file string) string {
	from := filepath.Join(test, file+".yaml")
	data, err := testfiles.Read(from)
	if err != nil {
		dir, _ := os.Getwd()
		klog.Infof("Maybe in wrong directory %s", dir)
		framework.Fail(err.Error())
	}
	return string(data)
}
