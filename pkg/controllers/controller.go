// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/mysql/ndb-operator/config/debug"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	ndbclientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
	ndbinformers "github.com/mysql/ndb-operator/pkg/generated/informers/externalversions/ndbcontroller/v1alpha1"
	ndblisters "github.com/mysql/ndb-operator/pkg/generated/listers/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/resources"
)

// ControllerContext summarizes the context in which it is running
type ControllerContext struct {
	// kubeClientset is the standard kubernetes clientset
	kubeClientset kubernetes.Interface
	ndbClientset  ndbclientset.Interface

	// runningInsideK8s is set to true if the operator is running inside a K8s cluster.
	runningInsideK8s bool
}

// NewControllerContext returns a new controller context object
func NewControllerContext(
	kubeclient kubernetes.Interface,
	ndbclient ndbclientset.Interface,
	runningInsideK8s bool,
) *ControllerContext {
	ctx := &ControllerContext{
		kubeClientset:    kubeclient,
		ndbClientset:     ndbclient,
		runningInsideK8s: runningInsideK8s,
	}

	return ctx
}

// Controller is the main controller implementation for Ndb resources
type Controller struct {
	controllerContext *ControllerContext

	statefulSetLister       appslisters.StatefulSetLister
	statefulSetListerSynced cache.InformerSynced

	ndbsLister ndblisters.NdbClusterLister
	ndbsSynced cache.InformerSynced

	mgmdController      StatefulSetControlInterface
	ndbdController      StatefulSetControlInterface
	configMapController ConfigMapControlInterface

	serviceLister       corelisters.ServiceLister
	serviceListerSynced cache.InformerSynced

	// deploymentLister is used to list all deployments
	deploymentLister       appslisters.DeploymentLister
	deploymentListerSynced cache.InformerSynced
	// The controller for the MySQL Server deployment run by the ndb operator
	mysqldController DeploymentControlInterface

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
	recorder events.EventRecorder
}

// NewController returns a new Ndb controller
func NewController(
	controllerContext *ControllerContext,
	statefulSetInformer appsinformers.StatefulSetInformer,
	deploymentInformer appsinformers.DeploymentInformer,
	serviceInformer coreinformers.ServiceInformer,
	podInformer coreinformers.PodInformer,
	configMapInformer coreinformers.ConfigMapInformer,
	ndbInformer ndbinformers.NdbClusterInformer) *Controller {

	statefulSetLister := statefulSetInformer.Lister()

	controller := &Controller{
		controllerContext:       controllerContext,
		ndbsLister:              ndbInformer.Lister(),
		ndbsSynced:              ndbInformer.Informer().HasSynced,
		statefulSetLister:       statefulSetLister,
		statefulSetListerSynced: statefulSetInformer.Informer().HasSynced,
		deploymentLister:        deploymentInformer.Lister(),
		deploymentListerSynced:  deploymentInformer.Informer().HasSynced,
		serviceLister:           serviceInformer.Lister(),
		serviceListerSynced:     serviceInformer.Informer().HasSynced,
		podLister:               podInformer.Lister(),
		podListerSynced:         podInformer.Informer().HasSynced,
		configMapController:     NewConfigMapControl(controllerContext.kubeClientset, configMapInformer),
		workqueue:               workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Ndbs"),
		recorder:                newEventRecorder(controllerContext.kubeClientset),

		mgmdController: NewRealStatefulSetControl(
			controllerContext.kubeClientset, statefulSetLister, resources.NewMgmdStatefulSet()),
		ndbdController: NewRealStatefulSetControl(
			controllerContext.kubeClientset, statefulSetLister, resources.NewNdbdStatefulSet()),
		mysqldController: NewMySQLDeploymentController(controllerContext.kubeClientset),
	}

	// Set up event handler for NdbCluster resource changes
	klog.Info("Setting up event handlers")
	ndbInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{

		AddFunc: func(obj interface{}) {
			ndb := obj.(*v1alpha1.NdbCluster)
			ndbKey := controller.getNdbClusterKey(ndb)
			klog.Infof("New NdbCluster resource added : %s", ndbKey)
			controller.workqueue.Add(ndbKey)
		},

		UpdateFunc: func(old, new interface{}) {
			oldNdb := old.(*v1alpha1.NdbCluster)
			ndbKey := controller.getNdbClusterKey(oldNdb)

			newNdb := new.(*v1alpha1.NdbCluster)
			if oldNdb.Generation != newNdb.Generation {
				// Spec of the NdbCluster resource was updated.
				klog.Infof("Spec of the NdbCluster resource '%s' was updated", ndbKey)
				klog.Infof("Generation updated from %d -> %d",
					oldNdb.Generation, newNdb.Generation)
				klog.Infof("Resource version updated from %s -> %s",
					oldNdb.ResourceVersion, newNdb.ResourceVersion)
			} else if oldNdb.ResourceVersion != newNdb.ResourceVersion {
				// Spec was not updated but the ResourceVersion changed => Status update
				klog.Infof("Status of the NdbCluster resource '%s' was updated", ndbKey)
				klog.Infof("Resource version updated from %s -> %s",
					oldNdb.ResourceVersion, newNdb.ResourceVersion)
				klog.Info("Nothing to do as only the status was updated.")
				return
			} else {
				// NdbCluster resource was not updated and this is a resync/requeue.
				klog.Infof("No updates to NdbCluster resource '%s'", ndbKey)
				if oldNdb.Generation != oldNdb.Status.ProcessedGeneration {
					// Controller is midway applying the previous change to NdbCluster resource.
					klog.Infof("Continuing reconciliation with generation %d", oldNdb.Generation)
				}
			}
			controller.workqueue.Add(ndbKey)
		},

		DeleteFunc: func(obj interface{}) {
			// Various K8s resources created and maintained for this NdbCluster
			// resource will have proper owner resources setup. Due to that, this
			// delete will automatically be cascaded to all those resources and
			// the controller doesn't have to do anything.
			ndb := obj.(*v1alpha1.NdbCluster)
			klog.Infof("NdbCluster resource '%s' was deleted", controller.getNdbClusterKey(ndb))
		},
	})

	return controller
}

// getNdbClusterKey returns a key for the
// given NdbCluster of form <namespace>/<name>.
func (c *Controller) getNdbClusterKey(nc *v1alpha1.NdbCluster) string {
	return nc.Namespace + "/" + nc.Name
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
		c.deploymentListerSynced,
		c.serviceListerSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	// Launch worker go routines to process Ndb resources
	for i := 0; i < threadiness; i++ {
		go wait.Until(func() {
			// The workers continue processing work items
			// available in the work queue until they are shutdown
			for c.processNextWorkItem() {
			}
		}, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

// processNextWorkItem reads a single work item off the
// workqueue and processes it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() (continueProcessing bool) {
	// Wait until there is a new item in the queue.
	// Get() also blocks other worker threads from
	// processing the 'item' until Done() is called on it.
	item, shutdown := c.workqueue.Get()
	if shutdown {
		return false
	}

	// Setup defer to call Done on the item to unblock it from other workers.
	defer c.workqueue.Done(item)

	// The item is a string key of the NdbCluster
	// resource object. It is of the form 'namespace/name'.
	key, ok := item.(string)
	if !ok {
		// item was not a string. Internal error.
		// Forget the item to avoid looping on it.
		c.workqueue.Forget(item)
		klog.Error(debug.InternalError(fmt.Errorf("expected string in workqueue but got %#v", item)))
		return true
	}

	// Run the syncHandler for the extracted key.
	klog.Infof("Starting a reconciliation cycle for NdbCluster resource %q", key)
	sr := c.syncHandler(key)
	klog.Infof("Completed a reconciliation cycle for NdbCluster resource %q", key)

	if err := sr.getError(); err != nil {
		klog.Infof("Reconciliation of NdbCluster resource %q failed", key)
		// The sync failed. It will be retried.
		klog.Info("Re-queuing resource to retry reconciliation after error")
		c.workqueue.AddRateLimited(key)
		return true
	}

	// The reconciliation loop was successful. Clear rateLimiter.
	c.workqueue.Forget(item)

	// Requeue the item if necessary
	if requeue, requeueInterval := sr.requeueSync(); requeue {
		klog.Infof("NdbCluster resource %q will be re-queued for further reconciliation", key)
		c.workqueue.AddAfter(item, requeueInterval)
	}

	return true
}

func (c *Controller) newSyncContext(ndb *v1alpha1.NdbCluster) *SyncContext {
	return &SyncContext{
		mgmdController:      c.mgmdController,
		ndbdController:      c.ndbdController,
		mysqldController:    c.mysqldController,
		configMapController: c.configMapController,
		ndb:                 ndb,
		controllerContext:   c.controllerContext,
		ndbsLister:          c.ndbsLister,
		recorder:            c.recorder,
	}
}

// syncHandler is the main reconciliation function
//   driving cluster towards desired configuration
//
// - synchronization happens in multiple steps
// - not all actions are taking in one call of syncHandler
//
// main principle:
// the desired state in Ndb CRD must be reflected in the Ndb configuration file
// and the cluster state will be adopted *to that config file first*
// before new changes from Ndb CRD are accepted
//
// Sync steps
//
// 1. ensure all resources are correctly created
// 2. ensure cluster is fully up and running and not in a degraded state
//    before rolling out any changes
// 3. drive cluster components towards the configuration previously
//    written to the configuration file
// 4. only after complete cluster is aligned with configuration file
//    new changes from Ndb CRD are written to a new version of the config file
// 5. update status of the CRD
func (c *Controller) syncHandler(key string) syncResult {

	klog.Infof("Sync handler: %s", key)

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		// no need to retry as this is a permanent error
		return finishProcessing()
	}

	// Get the NdbCluster resource with this namespace/name
	ndbOrg, err := c.ndbsLister.NdbClusters(namespace).Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Stop processing if the NdbCluster resource no longer exists
			klog.Infof("NdbCluster resource %q does not exist anymore", key)
			return finishProcessing()
		}

		klog.Infof("Failed to retrieve NdbCluster resource %q", key)
		return errorWhileProcessing(err)
	}

	// take a copy and process that for the update at the end
	// make all changes to the copy only and patch the original in the end
	ndb := ndbOrg.DeepCopy()

	syncContext := c.newSyncContext(ndb)

	return syncContext.sync(context.TODO())
}
