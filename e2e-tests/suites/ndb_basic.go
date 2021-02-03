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

	crd_utils "github.com/mysql/ndb-operator/e2e-tests/utils/crd"
	deployment_utils "github.com/mysql/ndb-operator/e2e-tests/utils/deployment"
	sfset_utils "github.com/mysql/ndb-operator/e2e-tests/utils/statefulset"
	yaml_utils "github.com/mysql/ndb-operator/e2e-tests/utils/yaml"

	"github.com/mysql/ndb-operator/pkg/constants"
	ndbclientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
)

var (
	deploySuite = []string{
		"helm/crds/mysql.oracle.com_ndbs",
	}

	ndbclient *ndbclientset.Clientset
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

	ndbclient, err = crd_utils.LoadClientset()
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

			sfset_utils.ExpectHasLabel(c, ns, "example-ndb-ndbd", constants.ClusterLabel, "example-ndb")
			sfset_utils.ExpectHasLabel(c, ns, "example-ndb-mgmd", constants.ClusterLabel, "example-ndb")

			sfset_utils.ExpectHasReplicas(c, ns, "example-ndb-mgmd", 2)
			sfset_utils.ExpectHasReplicas(c, ns, "example-ndb-ndbd", 2)

			deployment_utils.ExpectHasLabel(c, ns, "example-ndb-mysqld", constants.ClusterLabel, "example-ndb")

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

	framework.KubeDescribe("[Feature:basic error conditions]", func() {
		ginkgo.It("ndb basic error conditions", func() {

			var err error

			ndbobj := crd_utils.NewTestNdbCrd(ns, "test-ndb", 1, 2, 2)
			ndbobj, err = ndbclient.MysqlV1alpha1().Ndbs(ns).Create(context.TODO(), ndbobj, metav1.CreateOptions{})
			framework.ExpectNoError(err)

			err = ndbclient.MysqlV1alpha1().Ndbs(ns).Delete(context.TODO(), "test-ndb", metav1.DeleteOptions{})
			framework.ExpectNoError(err)
		})

	})
})
