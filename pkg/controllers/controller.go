// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	clientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
	samplescheme "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned/scheme"
	informers "github.com/mysql/ndb-operator/pkg/generated/informers/externalversions/ndbcontroller/v1alpha1"
	listers "github.com/mysql/ndb-operator/pkg/generated/listers/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/ndb"
	"github.com/mysql/ndb-operator/pkg/resources"
)

const controllerAgentName = "ndb-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a Foo is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a Foo fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by Foo"
	// MessageResourceSynced is the message used for an Event fired when a Foo
	// is synced successfully
	MessageResourceSynced = "Ndb synced successfully"
)

// Controller is the main controller implementation for Ndb resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface

	// ndbclientset is a clientset for our own API group
	ndbclientset clientset.Interface

	statefulSetLister       appslisters.StatefulSetLister
	statefulSetListerSynced cache.InformerSynced

	ndbsLister listers.NdbLister
	ndbsSynced cache.InformerSynced

	mgmdController      StatefulSetControlInterface
	ndbdController      StatefulSetControlInterface
	configMapController ConfigMapControlInterface

	serviceLister       corelisters.ServiceLister
	serviceListerSynced cache.InformerSynced

	// podLister is able to list/get Pods from a shared
	// informer's store.
	podLister corelisters.PodLister
	// podListerSynced returns true if the Pod shared informer
	// has synced at least once.
	podListerSynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new Ndb controller
func NewController(
	kubeclientset kubernetes.Interface,
	ndbclientset clientset.Interface,
	statefulSetInformer appsinformers.StatefulSetInformer,
	serviceInformer coreinformers.ServiceInformer,
	podInformer coreinformers.PodInformer,
	configMapInformer coreinformers.ConfigMapInformer,
	ndbInformer informers.NdbInformer) *Controller {

	// Create event broadcaster
	// Add ndb-controller types to the default Kubernetes Scheme so Events can be
	// logged for ndb-controller types.
	utilruntime.Must(samplescheme.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:           kubeclientset,
		ndbclientset:            ndbclientset,
		ndbsLister:              ndbInformer.Lister(),
		ndbsSynced:              ndbInformer.Informer().HasSynced,
		statefulSetLister:       statefulSetInformer.Lister(),
		statefulSetListerSynced: statefulSetInformer.Informer().HasSynced,
		serviceLister:           serviceInformer.Lister(),
		serviceListerSynced:     serviceInformer.Informer().HasSynced,
		podLister:               podInformer.Lister(),
		podListerSynced:         podInformer.Informer().HasSynced,
		configMapController:     NewConfigMapControl(kubeclientset, configMapInformer),
		workqueue:               workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Ndbs"),
		recorder:                recorder,
	}

	klog.Info("Setting up event handlers")
	// Set up an event handler for when Ndb resources change
	ndbInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueNdb,
		UpdateFunc: func(old, new interface{}) {
			oldNdb := old.(*v1alpha1.Ndb)
			newNdb := new.(*v1alpha1.Ndb)

			if oldNdb.ResourceVersion == newNdb.ResourceVersion {
				// we don't do anything here - just log
				// this can happen e.g. if the timer kicks in - desireable to check state once in the while
			} else {
				klog.Infof("Resource version: %s -> %s", oldNdb.ResourceVersion, newNdb.ResourceVersion)
			}

			klog.Infof("Generation: %d -> %d", oldNdb.ObjectMeta.Generation, newNdb.ObjectMeta.Generation)
			if !equality.Semantic.DeepEqual(oldNdb.Spec, newNdb.Spec) {
				klog.Infof("Difference in spec: %d : %d", *oldNdb.Spec.NodeCount, *newNdb.Spec.NodeCount)
			} else if !equality.Semantic.DeepEqual(oldNdb.Status, newNdb.Status) {
				klog.Infof("Difference in status")
			} else {
				klog.Infof("Other difference in spec")
				//diff.ObjectGoPrintSideBySide(oldNdb, newNdb))
			}

			controller.enqueueNdb(newNdb)
		},
		DeleteFunc: func(obj interface{}) {
			var ndb *v1alpha1.Ndb
			switch obj.(type) {
			case *v1alpha1.Ndb:
				ndb = obj.(*v1alpha1.Ndb)
			case cache.DeletedFinalStateUnknown:
				del := obj.(cache.DeletedFinalStateUnknown).Obj
				ndb = del.(*v1alpha1.Ndb)
			}

			if ndb != nil {
				klog.Infof("Delete object received and queued %s/%s", ndb.Namespace, ndb.Name)
				controller.enqueueNdb(ndb)
			} else {
				klog.Infof("Unkown deleted object ignored")
			}
		},
	})

	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*v1.Pod)
			ls := labels.Set(pod.Labels)
			if !ls.Has(constants.ClusterLabel) {
				return
			}
			//s, _ := json.MarshalIndent(pod.Status, "", "  ")
			//klog.Infof("%s", string(s))
			klog.Infof("pod new %s: phase= %s, ip=%s", pod.Name, pod.Status.Phase, pod.Status.PodIP)
		},
		UpdateFunc: func(old, new interface{}) {
			newPod := new.(*v1.Pod)
			ls := labels.Set(newPod.Labels)
			if !ls.Has(constants.ClusterLabel) {
				return
			}

			//oldPod := old.(*v1.Pod)
			//s, _ := json.MarshalIndent(newPod.Status, "", "  ")
			klog.Infof("pod upd %s: phase= %s, ip=%s", newPod.Name, newPod.Status.Phase, newPod.Status.PodIP)
		},
		DeleteFunc: func(obj interface{}) {
		},
	})

	statefulSetInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newStatefulSet := new.(*appsv1.StatefulSet)
			oldStatefulSet := old.(*appsv1.StatefulSet)
			if newStatefulSet.ResourceVersion == oldStatefulSet.ResourceVersion {
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	klog.Info("Starting Ndb controller")

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh,
		c.ndbsSynced,
		c.statefulSetListerSynced,
		c.serviceListerSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	// Launch two workers to process Ndb resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
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

func (c *Controller) updateClusterLabels(ndb *v1alpha1.Ndb, lbls labels.Set) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		ndb.Labels = labels.Merge(labels.Set(ndb.Labels), lbls)
		_, updateErr :=
			c.ndbclientset.NdbcontrollerV1alpha1().Ndbs(ndb.Namespace).Update(ndb)
		if updateErr == nil {
			return nil
		}

		key := fmt.Sprintf("%s/%s", ndb.GetNamespace(), ndb.GetName())
		klog.V(4).Infof("Conflict updating Cluster labels. Getting updated Cluster %s from cache...", key)

		updated, err := c.ndbsLister.Ndbs(ndb.Namespace).Get(ndb.Name)
		if err != nil {
			klog.Errorf("Error getting updated Cluster %q: %v", key, err)
			return err
		}

		// Copy the Cluster so we don't mutate the cache.
		ndb = updated.DeepCopy()
		return updateErr
	})
}

/* TODO function should ensure useful and needed default values for
the ndb setup */
func (c *Controller) ensureDefaults(ndb *v1alpha1.Ndb) {

}

func (c *Controller) ensureService(ndb *v1alpha1.Ndb, isMgmd bool, externalIP bool, name string) error {
	_, err := c.serviceLister.Services(ndb.Namespace).Get(name)
	if apierrors.IsNotFound(err) {
		klog.Infof("Creating a new Service for cluster %q",
			types.NamespacedName{Namespace: ndb.Namespace, Name: ndb.Name})
		svc := resources.NewService(ndb, isMgmd, externalIP, name)
		_, err = c.kubeclientset.CoreV1().Services(ndb.Namespace).Create(svc)
	}
	return err
}

func (c *Controller) ensureServices(ndb *v1alpha1.Ndb) error {
	err := c.ensureService(ndb, true, false, ndb.GetManagementServiceName())
	if err != nil {
		return err
	}
	err = c.ensureService(ndb, true, true, ndb.GetManagementServiceName()+"-ext")
	if err != nil {
		return err
	}
	err = c.ensureService(ndb, false, false, ndb.GetDataNodeServiceName())
	return err
}

func (c *Controller) ensurePodDisruptionBudget(ndb *v1alpha1.Ndb) error {
	pdbs := c.kubeclientset.PolicyV1beta1().PodDisruptionBudgets(ndb.Namespace)
	pdb, err := pdbs.Get(ndb.GetPodDisruptionBudgetName(), metav1.GetOptions{})
	if err != nil {
		klog.Errorf("error finding pdb: %s", err)
	}
	if apierrors.IsNotFound(err) {
		klog.Infof("Creating a new PodDisruptionBudget for Data Nodes of Cluster %q",
			types.NamespacedName{Namespace: ndb.Namespace, Name: ndb.Name})
		pdb = resources.NewPodDisruptionBudget(ndb)
		_, err = pdbs.Create(pdb)
	}
	return err
}

func (c *Controller) ensureManagementServerConfigVersion(ndbobj *v1alpha1.Ndb, wantedGeneration int) error {

	klog.Infof("Ensuring Management Server has correct config version")

	api := &ndb.Mgmclient{}

	// management server have the first nodeids
	// TODO: when we'll ever scale the number of management servers then this
	// needs to be changed to actually currently configured management servers
	// ndbobj has "desired" number of management servers
	for nodeID := 1; nodeID <= (int)(ndbobj.GetManagementNodeCount()); nodeID++ {

		// TODO : we use this function so far during test operator from outside cluster
		// we try connecting via load balancer until we connect to correct wanted node
		err := api.ConnectToNodeId(nodeID)
		if err != nil {
			klog.Errorf("No contact to management server to desired management server with node id %d established", nodeID)
			return err
		}

		defer api.Disconnect()

		version := api.GetConfigVersion()
		if version == wantedGeneration {
			klog.Infof("Management server with node id %d has desired version %d",
				nodeID, version)

			// node has right version, continue to process next node
			continue
		}

		klog.Infof("Management server with node id %d has different version %d than desired %d",
			nodeID, version, wantedGeneration)

		// check if management nodes report a degraded cluster state
		cs, err := api.GetStatus()
		if err != nil {
			klog.Errorf("Error getting cluster status from mangement server: %s", err)
		}

		if !cs.IsClusterDegraded() {

			klog.Infof("Cluster is reported to be fully running: attempting node stop of node %d", nodeID)

			// we are not in degraded in state
			// management server with nodeId was so nice to reveal all information
			// now we kill it - pod should terminate and restarted with updated config map and management server
			nodeIDs := []int{nodeID}
			_, err = api.StopNodes(&nodeIDs)

			// we do one at a time - exit here and wait for next reconcilation
			return nil
		}

		klog.Infof("Cluster is reported to be in degraded state: no restart attempted")
		return nil
	}

	return nil
}

func (c *Controller) rollingRestart(ndbobj *v1alpha1.Ndb) error {

	klog.Infof("Rolling restart")

	sel4ndb := labels.SelectorFromSet(ndbobj.GetLabels())
	pods, err := c.podLister.List(sel4ndb)
	if err != nil {
		return apierrors.NewNotFound(v1alpha1.Resource("Pod"), sel4ndb.String())
	}
	for _, pod := range pods {
		//url := fmt.Sprintf("http://%s:%d/ready", pod.Status.PodIP, 8080)

		// TODO for testing with external operator on minikube we start minikube tunnel and create a LoadBalancer
		url := fmt.Sprintf("http://%s:%d/ready", "127.0.0.1", 8080)
		resp, err := http.Get(url)
		if err != nil {
			klog.Errorf("error pinging pod %s %s: %s", pod.Name, pod.Status.PodIP, err)
		} else {
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				klog.Infof("pinging pod %s %s: no body", pod.Name, pod.Status.PodIP)
			} else {
				klog.Infof("pinging pod %s %s: %s", pod.Name, pod.Status.PodIP, body)
			}

		}
	}
	return nil
}

func (c *Controller) ensureClusterLabel(ndb *v1alpha1.Ndb) {
	// Ensure that the required labels are set on the cluster.
	sel4ndb := labels.SelectorFromSet(ndb.GetLabels())
	if !sel4ndb.Matches(labels.Set(ndb.Labels)) {
		klog.Infof("Setting labels on cluster %s", sel4ndb.String())
		c.updateClusterLabels(ndb.DeepCopy(), ndb.GetLabels())
	}
}

func (c *Controller) syncHandler(key string) error {

	klog.Infof("Sync handler: %s", key)

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}
	nsName := types.NamespacedName{Namespace: namespace, Name: name}

	// Get the Ndb resource with this namespace/name
	ndbOrg, err := c.ndbsLister.Ndbs(namespace).Get(name)
	if err != nil {
		klog.Infof("Ndb does not exist as resource, %s", name)
		// The Ndb resource may no longer exist, in which case we stop
		// processing.
		if apierrors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("ndb '%s' in work queue no longer exists", key))
			return nil
		}

		return err
	}

	// take a copy and process that for the update at the end
	// make all changes to the copy only and patch the original in the end
	ndb := ndbOrg.DeepCopy()

	// TODO - not sure if we need a cluster level label on the CRD
	//      causes an update event looping us in here again
	c.ensureClusterLabel(ndb)

	// first get a hash of the new config to see if anything changed
	configHash, equalConfig, err := ndb.IsConfigHashEqual()
	if err != nil {
		return err
	}

	//
	if equalConfig {
		klog.Infof("No update to configuration detected in %s based on hash %s",
			nsName, configHash)
		// just using it for fyi at the moment
		// still need to continue here as not all nodes might be on that version yet
	} else {
		klog.Infof("Configuration change detected in %s based on hash", nsName)
		klog.Infof("org: %x %d %s", ndb.Status.ReceivedConfigHash, ndb.Status.ProcessedGeneration, ndb.Status.LastUpdate)
		ndb.Status.ReceivedConfigHash = configHash
		klog.Infof("new: %x", configHash)
	}

	if err := c.ensureServices(ndb); err != nil {
		// re-queue if something went wrong
		return err
	}
	if err := c.ensurePodDisruptionBudget(ndb); err != nil {
		// re-queue if something went wrong
		return err
	}

	// create config map if not exist
	cm, err := c.configMapController.EnsureConfigMap(ndb)
	if err != nil {
		return err
	}

	// create the management stateful set if it doesn't exist
	if c.mgmdController == nil {
		mgmdSfSet := resources.NewMgmdStatefulSet(ndb)
		c.mgmdController =
			&realStatefulSetControl{
				client:            c.kubeclientset,
				statefulSetLister: c.statefulSetLister,
				statefulSetType:   mgmdSfSet}
	}

	sfset, err := c.mgmdController.EnsureStatefulSet(ndb)

	if err != nil {
		return err
	}

	// create the data node stateful set if it doesn't exist
	if c.ndbdController == nil {
		ndbdSfSet := resources.NewNdbdStatefulSet(ndb)
		c.ndbdController =
			&realStatefulSetControl{
				client:            c.kubeclientset,
				statefulSetLister: c.statefulSetLister,
				statefulSetType:   ndbdSfSet}
	}

	sfset, err = c.ndbdController.EnsureStatefulSet(ndb)
	if err != nil {
		return err
	}

	// If the StatefulSet is not controlled by this Ndb resource, we should log
	// a warning to the event recorder and return error msg.
	if !metav1.IsControlledBy(sfset, ndb) {
		msg := fmt.Sprintf(MessageResourceExists, sfset.Name)
		c.recorder.Event(ndb, corev1.EventTypeWarning, ErrResourceExists, msg)
		return fmt.Errorf(msg)
	}

	// if the config map exists then check if its up-to-date

	// get config string
	configString, err := c.configMapController.ExtractConfig(cm)
	if err != nil {
		return err
	}
	//klog.Infof("Config string %s\n", configString)

	hash, generation, err := resources.GetConfigHashAndGenerationFromConfig(configString)
	if err != nil {
		klog.Errorf("Extracting hash or config generation from configuration failed. hash= %s, gen= %d",
			hash, generation)
		return err
	}
	klog.Infof("Extracting hash or config generation from configuration: hash= %s, gen= %d",
		hash, generation)

	if hash != configHash {
		klog.Infof("Config received is different from config map config. config map: \"%s\", new: \"%s\"",
			hash, configHash)

		_, err := c.configMapController.PatchConfigMap(ndb)
		if err != nil {
			klog.Infof("Failed to patch config map")
			return err
		}
	}
	if ndb.Status.ProcessedGeneration != ndb.ObjectMeta.Generation {
		klog.Infof("Config generation received different from config map generation. config map: %d, new: %d",
			generation, ndb.ObjectMeta.Generation)
	}

	c.ensureManagementServerConfigVersion(ndb, int(generation))

	// If this number of the members on the Cluster does not equal the
	// current desired replicas on the StatefulSet, we should update the
	// StatefulSet resource.
	if *ndb.Spec.NodeCount != *sfset.Spec.Replicas {
		klog.Infof("Updating %q: DataNodes=%d statefulSetReplicas=%d",
			nsName, *ndb.Spec.NodeCount, *sfset.Spec.Replicas)
		if sfset, err = c.ndbdController.Patch(ndb, sfset); err != nil {
			// Requeue the item so we can attempt processing again later.
			// This could have been caused by a temporary network failure etc.
			return err
		}
	}

	if ndb.Status.ProcessedGeneration != ndb.ObjectMeta.Generation {
		c.rollingRestart(ndb)
	}

	// Finally, we update the status block of the Ndb resource to reflect the
	// current state of the world
	err = c.updateNdbStatus(ndb)
	if err != nil {
		klog.Errorf("updating status failed: %v", err)
		return err
	}

	//c.podListing(ndb)
	klog.Infof("Returning from syncHandler")

	c.recorder.Event(ndb, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func updatePodForTest(pod *v1.Pod) *v1.Pod {
	t := time.Now()

	ann := map[string]string{
		"test": t.Format(time.UnixDate),
	}

	pod.Annotations = ann
	/*
		for idx, container := range pod.Spec.Containers {
			if container.Name == targetContainer {
				pod.Spec.Containers[idx].Image = newAgentImage
				break
			}
		}
	*/
	return pod
}

// PatchPod perform a direct patch update for the specified Pod.
func patchPod(kubeClient kubernetes.Interface, oldData *corev1.Pod, newData *corev1.Pod) (*corev1.Pod, error) {
	currentPodJSON, err := json.Marshal(oldData)
	if err != nil {
		return nil, err
	}

	updatedPodJSON, err := json.Marshal(newData)
	if err != nil {
		return nil, err
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(currentPodJSON, updatedPodJSON, corev1.Pod{})
	if err != nil {
		return nil, err
	}
	klog.V(4).Infof("Patching Pod %q: %s", types.NamespacedName{Namespace: oldData.Namespace, Name: oldData.Name}, string(patchBytes))

	result, err := kubeClient.CoreV1().Pods(oldData.Namespace).Patch(oldData.Name, types.StrategicMergePatchType, patchBytes)
	if err != nil {
		return nil, apierrors.NewNotFound(v1alpha1.Resource("Pod"), "failed patching pod")
	}

	return result, nil
}

func (c *Controller) podListing(ndb *v1alpha1.Ndb) error {

	sel4ndb := labels.SelectorFromSet(ndb.GetLabels())
	pods, err := c.podLister.List(sel4ndb)
	if err != nil {
		return apierrors.NewNotFound(v1alpha1.Resource("Pod"), sel4ndb.String())
	}
	for _, pod := range pods {
		//klog.Infof("Ndb pod '%s/%s'", pod.Namespace, pod.Name)
		newPod := updatePodForTest(pod.DeepCopy())
		pod, err = patchPod(c.kubeclientset, pod, newPod)
		if err != nil {
			return apierrors.NewNotFound(v1alpha1.Resource("Pod"), "upgrade operator version: PatchPod failed")
		}

	}

	return nil
}

func (c *Controller) updateNdbStatus(ndb *v1alpha1.Ndb) error {

	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance

	// we already received a copy here

	updateErr := wait.ExponentialBackoff(retry.DefaultBackoff, func() (ok bool, err error) {

		klog.Infof("Updating ndb cluster status")

		ndb.Status.LastUpdate = metav1.NewTime(time.Now())
		ndb.Status.ProcessedGeneration = ndb.ObjectMeta.Generation

		// If the CustomResourceSubresources feature gate is not enabled,
		// we must use Update instead of UpdateStatus to update the Status block of the Ndb resource.
		// UpdateStatus will not allow changes to the Spec of the resource,
		// which is ideal for ensuring nothing other than resource status has been updated.
		//_, err = c.ndbclientset.NdbcontrollerV1alpha1().Ndbs(ndb.Namespace).Update(ndb)

		_, err = c.ndbclientset.NdbcontrollerV1alpha1().Ndbs(ndb.Namespace).UpdateStatus(ndb)
		if err == nil {
			return true, nil
		}
		if !errors.IsConflict(err) {
			return false, err
		}

		updated, err := c.ndbclientset.NdbcontrollerV1alpha1().Ndbs(ndb.Namespace).Get(ndb.Name, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("failed to get Ndb %s/%s: %v", ndb.Namespace, ndb.Name, err)
			return false, err
		}
		ndb = updated.DeepCopy()
		return false, nil
	})

	if updateErr != nil {
		klog.Errorf("failed to update Ndb %s/%s: %v", ndb.Namespace, ndb.Name, updateErr)
		return updateErr
	}
	return nil
}

func (c *Controller) onDeleteNdb(ndb *v1alpha1.Ndb) {}

/*
	enqueueNdb takes a Ndb resource and converts it into a namespace/name
   	string which is then put onto the work queue. This method should *not* be
   	passed resources of any type other than Ndb.
*/
func (c *Controller) enqueueNdb(obj interface{}) {
	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	klog.Infof("Processing Ndb: %s", key)
	c.workqueue.Add(key)
}

/*
   handleObject will take any resource implementing metav1.Object and attempt
   to find the Ndb resource that 'owns' it. It does this by looking at the
   objects metadata.ownerReferences field for an appropriate OwnerReference.
   It then enqueues that Foo resource to be processed. If the object does not
   have an appropriate OwnerReference, it will simply be skipped.
*/
func (c *Controller) handleObject(obj interface{}) {
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
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a Ndb, we should not do anything more
		// with it.
		if ownerRef.Kind != "Ndb" {
			return
		}

		ndb, err := c.ndbsLister.Ndbs(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			klog.Infof("ignoring orphaned object '%s' of ndb '%s'", object.GetSelfLink(), ownerRef.Name)
			return
		}

		klog.Infof("Ignoring object: %s", ndb.GetName())
		//c.enqueueNdb(ndb)
		return
	}
}
