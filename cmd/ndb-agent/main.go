package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/mysql/ndb-operator/pkg/controllers/agent"
	clientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
	informers "github.com/mysql/ndb-operator/pkg/generated/informers/externalversions"
	"github.com/mysql/ndb-operator/pkg/signals"
	"github.com/mysql/ndb-operator/pkg/version"

	kubeinformers "k8s.io/client-go/informers"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

var (
	masterURL  string
	kubeconfig string
)

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
}

func handler(w http.ResponseWriter, r *http.Request) {
	name, _ := os.Hostname()
	fmt.Fprintf(w, "Hi there from %s!\n", name)
}
func agent_main() {

	flag.Parse()

	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %s", err.Error())
	}
	/* just testing for readiness probes */
	go func() {
		http.HandleFunc("/", handler)
		http.HandleFunc("/live", handler)
		http.HandleFunc("/ready", handler)
		klog.Fatal(http.ListenAndServe(":8080", nil))
	}()

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	ndbClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building ndb clientset: %s", err.Error())
	}

	k8If := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	ndbOpIf := informers.NewSharedInformerFactory(ndbClient, time.Second*30)

	agentVersion := version.GetBuildVersion()
	klog.Infof("Starting agent version \"%s\"", agentVersion)

	ag, err := agent.New(kubeClient, ndbClient,
		k8If.Core().V1().Pods(),
		ndbOpIf.Mysql().V1alpha1().Ndbs())

	k8If.Start(stopCh)
	ndbOpIf.Start(stopCh)

	if err = ag.Run(stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}
}

func main() {
	agent_main()
}
