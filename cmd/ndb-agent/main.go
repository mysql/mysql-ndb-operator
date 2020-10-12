package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/ocklin/ndb-operator/pkg/signals"
	"github.com/ocklin/ndb-operator/pkg/version"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
)

const ClusterLabel = "ndbcontroller.mysql.com/v1alpha1"

func looper() {

	for {
		name, err := os.Hostname()
		if err != nil {
			panic(err)
		}
		addrs, err := net.LookupIP(name)
		if err != nil {
		}

		for _, addr := range addrs {
			ipv4 := addr.To4()
			if ipv4 != nil && len(ipv4) > 0 {
				hosts, err := net.LookupAddr(ipv4.String())
				if err != nil || len(hosts) == 0 {
					break
				}
				fqdn := hosts[0]
				fmt.Println("hostname:", name, fqdn)
			}
		}

		time.Sleep(1000 * time.Millisecond)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	name, _ := os.Hostname()
	fmt.Fprintf(w, "Hi there from %s!\n", name)
}

var (
	masterURL  string
	kubeconfig string
)

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
}

func change(obj interface{}) {
	fmt.Printf("something changed\n")
}

type Agent struct {
	lister corelisters.PodLister
	synced cache.InformerSynced

	workqueue workqueue.RateLimitingInterface
}

func New(clientset kubernetes.Interface, namespace string,
	podIf v1.PodInformer) *Agent {

	ag := &Agent{
		lister:    podIf.Lister(),
		synced:    podIf.Informer().HasSynced,
		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "ndb-agent"),
	}

	podIf.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			fmt.Println("pod added")
			ag.handler(obj)
		},
		UpdateFunc: func(old, new interface{}) {
			fmt.Println("pod updated")
			ag.handler(new)
		},
		DeleteFunc: func(obj interface{}) {
			fmt.Println("pod deleted")
			ag.handler(obj)
		},
	})

	return ag
}

func (c *Agent) handler(obj interface{}) {
	fmt.Println("handler")
	var object metav1.Object
	var ok bool
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		klog.V(4).Infof("Recovered deleted object '%s' from tombstone", object.GetName())
	}
	klog.Infof("Processing object: %s", object.GetName())

	/*
		currentPodJSON, err := json.MarshalIndent(object, "", "  ")
		if err != nil {
			return
		}
		klog.Infof(string(currentPodJSON))
	*/

	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a Ndb, we should not do anything more
		// with it.
		klog.Infof("Owner kind: %s, name: %s", ownerRef.Kind, ownerRef.Name)
		if ownerRef.Kind != "StatefulSet" {
			return
		}
		//c.enqueueNdb(ndb)
		return
	}

	if pod, ok := obj.(corev1.Pod); !ok {
		klog.Infof("Processing object: %s", pod.GetName())
	}
}

func (c *Agent) Run(stopCh <-chan struct{}) error {

	defer runtime.HandleCrash()

	klog.Info("starting controller")
	defer klog.Info("shutting down controller ")

	if ok := cache.WaitForCacheSync(stopCh, c.synced); !ok {
		klog.Info("failed to wait for caches to sync")
		return nil
	}

	klog.Info("Starting workers")
	// Launch worker to process Ndb resources
	go wait.Until(c.runWorker, time.Second, stopCh)

	<-stopCh

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Agent) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Agent) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		klog.Infof("Working on '%s'", key)
		// Run the syncHandler, passing it the namespace/name string of the
		// Foo resource to be synced.
		if err := c.syncHandler(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

func (c *Agent) syncHandler(key string) error {
	klog.Infof("sync handler")
	return nil
}

func oneStyleLister(clientset kubernetes.Interface) {

	sel4ndb := metav1.ListOptions{
		//FieldSelector: "metadata.name=ndb-ndbd-service",
		LabelSelector: ClusterLabel,
	}

	eps, err := clientset.CoreV1().Endpoints("default").List(sel4ndb)

	for i, ep := range eps.Items {
		// full list is a cartesian product of addresses x ports
		for j, s := range ep.Subsets {
			for _, a := range s.Addresses {
				for l, p := range s.Ports {
					fmt.Printf("%d %d %d %s/%s %s %s:%d %s\n",
						i, j, l, ep.GetNamespace(), ep.GetName(),
						a.Hostname, a.IP, p.Port, p.Protocol)
				}
			}
		}
	}

	if err != nil {
		// re-queue if something went wrong
		return
	}
}

func main() {

	flag.Parse()
	namespace := "default"

	stopCh := signals.SetupSignalHandler()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	informerFactory := informers.NewFilteredSharedInformerFactory(kubeClient, time.Second*30, namespace, nil)
	pods := informerFactory.Core().V1().Pods()

	ag := New(kubeClient, namespace, pods)

	informerFactory.Start(stopCh)

	if err = ag.Run(stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}

	//	select {}

	agentVersion := version.GetBuildVersion()

	klog.Infof("Starting agent version \"%s\"", agentVersion)

	http.HandleFunc("/", handler)
	klog.Fatal(http.ListenAndServe(":8080", nil))
}
