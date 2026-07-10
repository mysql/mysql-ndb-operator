// Copyright (c) 2020, 2026, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/workqueue"
	klog "k8s.io/klog/v2"

	"github.com/mysql/ndb-operator/config/debug"
	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
	ndbclientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
	ndbinformers "github.com/mysql/ndb-operator/pkg/generated/informers/externalversions"
	ndblisters "github.com/mysql/ndb-operator/pkg/generated/listers/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/helpers"
)

// Controller is the main controller implementation for Ndb resources
type Controller struct {
	kubernetesClient kubernetes.Interface
	ndbClient        ndbclientset.Interface

	// NdbCluster Lister
	ndbsLister ndblisters.NdbClusterLister

	// Controllers for various resources
	mgmdController           *ndbNodeStatefulSetImpl
	ndbmtdController         *ndbmtdStatefulSetController
	mysqldController         *mysqldStatefulSetController
	configMapController      ConfigMapControlInterface
	serviceController        ServiceControlInterface
	serviceAccountController ServiceAccountControlInterface
	pdbController            PodDisruptionBudgetControlInterface
	networkPolicyController  NetworkPolicyControlInterface

	// K8s Listers
	podLister     corelisters.PodLister
	pvcLister     corelisters.PersistentVolumeClaimLister
	serviceLister corelisters.ServiceLister

	// Slice of InformerSynced methods for all the informers used by the controller
	informerSyncedMethods []cache.InformerSynced

	// A rate limited workqueue for queueing the NdbCluster resource
	// keys on receiving an event. The workqueue ensures that the same
	// key is not processed simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// An event recorder for recording Event resources to the Kubernetes API.
	recorder events.EventRecorder
}

// NewController returns a new Ndb controller
func NewController(
	kubernetesClient kubernetes.Interface,
	ndbClient ndbclientset.Interface,
	k8sSharedIndexInformer kubeinformers.SharedInformerFactory,
	ndbSharedIndexInformer ndbinformers.SharedInformerFactory) *Controller {

	// Register for all the required informers
	ndbClusterInformer := ndbSharedIndexInformer.Mysql().V1().NdbClusters()
	statefulSetInformer := k8sSharedIndexInformer.Apps().V1().StatefulSets()
	podInformer := k8sSharedIndexInformer.Core().V1().Pods()
	serviceInformer := k8sSharedIndexInformer.Core().V1().Services()
	configmapInformer := k8sSharedIndexInformer.Core().V1().ConfigMaps()
	secretInformer := k8sSharedIndexInformer.Core().V1().Secrets()
	serviceAccountInformer := k8sSharedIndexInformer.Core().V1().ServiceAccounts()
	pvcInformer := k8sSharedIndexInformer.Core().V1().PersistentVolumeClaims()
	networkPolicyInformer := k8sSharedIndexInformer.Networking().V1().NetworkPolicies()

	// Extract all the InformerSynced methods
	informerSyncedMethods := []cache.InformerSynced{
		ndbClusterInformer.Informer().HasSynced,
		statefulSetInformer.Informer().HasSynced,
		podInformer.Informer().HasSynced,
		serviceInformer.Informer().HasSynced,
		configmapInformer.Informer().HasSynced,
		secretInformer.Informer().HasSynced,
		serviceAccountInformer.Informer().HasSynced,
		pvcInformer.Informer().HasSynced,
		networkPolicyInformer.Informer().HasSynced,
	}

	serviceLister := serviceInformer.Lister()
	statefulSetLister := statefulSetInformer.Lister()
	configmapLister := configmapInformer.Lister()
	secretLister := secretInformer.Lister()
	serviceAccountLister := serviceAccountInformer.Lister()
	networkPolicyLister := networkPolicyInformer.Lister()

	// The operator peer must be restricted to the namespace where it actually runs.
	operatorNamespace, err := helpers.GetCurrentNamespace()
	if err != nil {
		klog.Warningf("Failed to determine operator namespace for NetworkPolicy generation: %s", err)
	}

	controller := &Controller{
		kubernetesClient:         kubernetesClient,
		ndbClient:                ndbClient,
		informerSyncedMethods:    informerSyncedMethods,
		ndbsLister:               ndbClusterInformer.Lister(),
		podLister:                podInformer.Lister(),
		pvcLister:                pvcInformer.Lister(),
		configMapController:      NewConfigMapControl(kubernetesClient, configmapLister),
		serviceController:        NewServiceControl(kubernetesClient, serviceLister),
		serviceAccountController: NewServiceAccountControl(kubernetesClient, serviceAccountLister),
		networkPolicyController:  newNetworkPolicyControl(kubernetesClient, networkPolicyLister, operatorNamespace),
		workqueue:                workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Ndbs"),
		recorder:                 newEventRecorder(kubernetesClient),

		mgmdController:   newMgmdStatefulSetController(kubernetesClient, statefulSetLister),
		ndbmtdController: newNdbmtdStatefulSetController(kubernetesClient, statefulSetLister, secretLister),
		mysqldController: newMySQLDStatefulSetController(
			kubernetesClient, statefulSetLister, configmapLister),
	}

	// Setup informer and controller for v1.PDB if K8s Server has the support
	if ServerSupportsV1Policy(kubernetesClient) {
		pdbInformer := k8sSharedIndexInformer.Policy().V1().PodDisruptionBudgets()
		controller.informerSyncedMethods = append(controller.informerSyncedMethods, pdbInformer.Informer().HasSynced)
		controller.pdbController = newPodDisruptionBudgetControl(kubernetesClient, pdbInformer.Lister())
	}

	klog.Info("Setting up event handlers")
	// Set up event handler for NdbCluster resource changes
	ndbClusterInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{

		AddFunc: func(obj interface{}) {
			ndb := obj.(*v1.NdbCluster)
			ndbKey := getNdbClusterKey(ndb)
			klog.Infof("New NdbCluster resource added : %q, queueing it for reconciliation", ndbKey)
			controller.workqueue.Add(ndbKey)
		},

		UpdateFunc: func(old, new interface{}) {
			oldNdb := old.(*v1.NdbCluster)
			ndbKey := getNdbClusterKey(oldNdb)

			newNdb := new.(*v1.NdbCluster)
			if oldNdb.Generation != newNdb.Generation {
				// Spec of the NdbCluster resource was updated.
				klog.Infof("Spec of the NdbCluster resource %q was updated", ndbKey)
				klog.Infof("Generation updated from %d -> %d",
					oldNdb.Generation, newNdb.Generation)
				klog.Infof("Resource version updated from %s -> %s",
					oldNdb.ResourceVersion, newNdb.ResourceVersion)
				klog.Infof("NdbCluster resource %q is added to the queue for reconciliation", ndbKey)
			} else if managementNetworkPolicyAnnotationChanged(oldNdb, newNdb) {
				klog.Infof("Management NetworkPolicy annotation of NdbCluster resource %q was updated", ndbKey)
				klog.Infof("Resource version updated from %s -> %s",
					oldNdb.ResourceVersion, newNdb.ResourceVersion)
				controller.recordManagementAccessWarnings(newNdb)
				klog.Infof("NdbCluster resource %q is added to the queue for reconciliation", ndbKey)
			} else if oldNdb.ResourceVersion != newNdb.ResourceVersion {
				// Spec was not updated but the ResourceVersion changed => Status update
				klog.V(2).Infof("Status of the NdbCluster resource '%s' was updated", ndbKey)
				klog.V(2).Infof("Resource version updated from %s -> %s",
					oldNdb.ResourceVersion, newNdb.ResourceVersion)
				klog.V(2).Info("Nothing to do as only the status was updated.")
				return
			} else {
				// NdbCluster resource was not updated and this is a resync/requeue.
				if oldNdb.Generation != oldNdb.Status.ProcessedGeneration {
					// Reconciliation is already underway. Either it is being handled by a
					// worker right now or the controller is waiting for some resource to get
					// ready, and it will continue once it is ready.
					// So, no need to add the item to the workqueue now.
					return
				}
				klog.Infof("No updates to NdbCluster resource %q, queueing it for periodic reconciliation", ndbKey)
			}
			controller.workqueue.Add(ndbKey)
		},

		DeleteFunc: func(obj interface{}) {
			// Various K8s resources created and maintained for this NdbCluster
			// resource will have proper owner resources setup. Due to that, this
			// delete will automatically be cascaded to all those resources and
			// the controller doesn't have to do anything.
			ndb := obj.(*v1.NdbCluster)

			klog.Infof("NdbCluster resource '%s' was deleted", getNdbClusterKey(ndb))

			// List all pvcs owned by NdbCluster resource
			pvcs, listErr := controller.pvcLister.List(labels.Set(ndb.GetLabels()).AsSelector())
			if listErr != nil {
				klog.Errorf("Failed to list pvc's owned by NdbCluster %q : %s", getNamespacedName(ndb), listErr)
			}

			// Delete all pvc
			for _, pvc := range pvcs {
				err := controller.kubernetesClient.CoreV1().PersistentVolumeClaims(ndb.Namespace).Delete(context.Background(), pvc.Name, metav1.DeleteOptions{})
				if err != nil && !errors.IsNotFound(err) {
					// Delete failed with an error.
					klog.Errorf("Failed to delete pvc %q : %s", pvc, err)
				}
			}
		},
	})

	// Set up event handlers for StatefulSet resource changes
	statefulSetInformer.Informer().AddEventHandlerWithResyncPeriod(

		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				// Filter out all StatefulSets not owned by any
				// NdbCluster resources. The StatefulSet labels
				// will have the names of their respective
				// NdbCluster owners.
				sfset := obj.(*appsv1.StatefulSet)
				_, clusterLabelExists := sfset.GetLabels()[constants.ClusterLabel]
				return clusterLabelExists
			},

			Handler: cache.ResourceEventHandlerFuncs{
				// When a StatefulSet owned by a NdbCluster resource
				// is updated, either
				//  a) it is ready, in which case, start the next
				//     reconciliation loop (or)
				//  b) one or more pods status have become ready/unready,
				//     in which case the status of the NdbCluster resource
				//     needs to be updated, which also happens through a
				//     reconciliation loop.
				//  So, add the NdbCluster item to the workqueue for both cases.
				UpdateFunc: func(oldObj, newObj interface{}) {
					oldStatefulSet := oldObj.(*appsv1.StatefulSet)
					newStatefulSet := newObj.(*appsv1.StatefulSet)

					if reflect.DeepEqual(oldStatefulSet.Status, newStatefulSet.Status) {
						// No updates to status => this event was triggered
						// by an update to the StatefulSet spec by the operator.
						// No need to enqueue for reconciliation.
						return
					}

					if oldStatefulSet.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
						oldStatus, newStatus := oldStatefulSet.Status, newStatefulSet.Status
						if (oldStatus.CurrentReplicas != newStatus.CurrentReplicas ||
							oldStatus.CurrentRevision != newStatus.CurrentRevision) &&
							oldStatus.UpdatedReplicas == newStatus.UpdatedReplicas &&
							oldStatus.UpdateRevision == newStatus.UpdateRevision &&
							oldStatus.ReadyReplicas == newStatus.ReadyReplicas &&
							oldStatus.Replicas == newStatus.Replicas &&
							oldStatus.ObservedGeneration == newStatus.ObservedGeneration {
							// https://github.com/kubernetes/kubernetes/issues/106055
							// CurrentRevision not updated if OnDelete update strategy is used.
							// But the CurrentReplicas get erroneously updated on some occasions.
							// Ignore any status updated made only to those fields if the
							// UpdateStrategy is OnDelete.
							return
						}
					}

					controller.extractAndEnqueueNdbCluster(newStatefulSet, "StatefulSet", "updated")
				},

				// When a statefulset owned by a NdbCluster resource
				// is deleted, add the NdbCluster resource to the
				// workqueue to start the next reconciliation loop.
				DeleteFunc: func(obj interface{}) {
					sfset := obj.(*appsv1.StatefulSet)
					klog.Infof("StatefulSet %q is deleted", getNamespacedName(sfset))
					controller.extractAndEnqueueNdbCluster(sfset, "StatefulSet", "deleted")
				},
			},
		},

		// Set resyncPeriod to 0 to ignore all re-sync events
		0,
	)

	// Set up event handlers for ConfigMap updates
	configmapInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				// Filter out all ConfigMaps not owned by any NdbCluster resources.
				// The ConfigMap labels will have the names of their respective
				// NdbCluster owners.
				configmap := obj.(*corev1.ConfigMap)
				_, clusterLabelExists := configmap.GetLabels()[constants.ClusterLabel]
				return clusterLabelExists
			},

			Handler: cache.ResourceEventHandlerFuncs{
				// A ConfigMap owned by an NdbCluster object was updated
				// Requeue owner for reconciliation
				UpdateFunc: func(oldObj, newObj interface{}) {
					newConfigMap := newObj.(*corev1.ConfigMap)
					controller.extractAndEnqueueNdbCluster(newConfigMap, "ConfigMap", "updated")
				},
			},
		},

		// Set resyncPeriod to 0 to ignore all re-sync events
		0,
	)

	// Repair policy drift or deletion without waiting for an NdbCluster change.
	networkPolicyInformer.Informer().AddEventHandlerWithResyncPeriod(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				networkPolicy, ok := networkPolicyFromInformerObject(obj)
				if !ok {
					return false
				}
				_, clusterLabelExists := networkPolicy.GetLabels()[constants.ClusterLabel]
				return clusterLabelExists
			},

			Handler: cache.ResourceEventHandlerFuncs{
				UpdateFunc: func(oldObj, newObj interface{}) {
					oldNetworkPolicy := oldObj.(*networkingv1.NetworkPolicy)
					newNetworkPolicy := newObj.(*networkingv1.NetworkPolicy)

					if reflect.DeepEqual(oldNetworkPolicy.Spec, newNetworkPolicy.Spec) &&
						reflect.DeepEqual(oldNetworkPolicy.Labels, newNetworkPolicy.Labels) &&
						reflect.DeepEqual(oldNetworkPolicy.OwnerReferences, newNetworkPolicy.OwnerReferences) {
						return
					}

					// Ownership is authoritative; mutable labels must not redirect reconciliation.
					oldNdbClusterName := getOwningNdbClusterName(oldNetworkPolicy)
					newNdbClusterName := getOwningNdbClusterName(newNetworkPolicy)
					controller.extractAndEnqueueNdbCluster(oldNetworkPolicy, "NetworkPolicy", "updated")
					if oldNetworkPolicy.Namespace != newNetworkPolicy.Namespace ||
						oldNdbClusterName != newNdbClusterName {
						controller.extractAndEnqueueNdbCluster(newNetworkPolicy, "NetworkPolicy", "updated")
					}
				},

				// Requeue the owner so a deleted security policy is recreated.
				DeleteFunc: func(obj interface{}) {
					networkPolicy, ok := networkPolicyFromInformerObject(obj)
					if !ok {
						return
					}
					klog.Infof("NetworkPolicy %q is deleted", getNamespacedName(networkPolicy))
					controller.extractAndEnqueueNdbCluster(networkPolicy, "NetworkPolicy", "deleted")
				},
			},
		},

		// Set resyncPeriod to 0 to ignore all re-sync events
		0,
	)

	// Set up event handlers for Pod Status changes
	podInformer.Informer().AddEventHandlerWithResyncPeriod(

		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				// Filter out all Pods not owned by any NdbCluster resources.
				// The Pod labels will have the names of their respective
				// NdbCluster owners.
				pod := obj.(*corev1.Pod)
				_, clusterLabelExists := pod.GetLabels()[constants.ClusterLabel]
				return clusterLabelExists
			},

			Handler: cache.ResourceEventHandlerFuncs{
				// When a pod owned by an NdbCluster resource fails or
				// recovers from an error, the NdbCluster status needs to be updated.
				UpdateFunc: func(oldObj, newObj interface{}) {
					oldPod := oldObj.(*corev1.Pod)
					newPod := newObj.(*corev1.Pod)

					if !reflect.DeepEqual(getPodErrors(oldPod), getPodErrors(newPod)) {
						// The error status of the Pod has changed.
						controller.extractAndEnqueueNdbCluster(newPod, "Pod", "updated")
					}
				},
			},
		},

		// Set resyncPeriod to 0 to ignore all re-sync events
		0,
	)

	return controller
}

// networkPolicyFromInformerObject unwraps deletion tombstones so a missed watch
// event cannot crash policy reconciliation.
func networkPolicyFromInformerObject(obj interface{}) (*networkingv1.NetworkPolicy, bool) {
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	networkPolicy, ok := obj.(*networkingv1.NetworkPolicy)
	if !ok {
		klog.Errorf("Expected NetworkPolicy informer object, got %T", obj)
		return nil, false
	}

	return networkPolicy, true
}

// ndbClusterExists returns true if a NdbCluster object exists with the given namespace/name
func (c *Controller) ndbClusterExists(namespace, name string) (bool, error) {
	_, err := c.ndbsLister.NdbClusters(namespace).Get(name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// NdbCluster doesn't exists
			return false, nil
		}

		// Some error occurred when Getting NdbCluster
		klog.Errorf("Failed to retrieve NdbCluster resource %q", getNamespacedName2(namespace, name))
		return false, err
	}

	return true, nil
}

// getOwningNdbClusterName prefers controller ownership metadata over routing
// labels.
func getOwningNdbClusterName(obj metav1.Object) string {
	// Indirectly owned resources such as Pods still require the cluster label.
	controllerRef := metav1.GetControllerOf(obj)
	if controllerRef != nil &&
		controllerRef.APIVersion == v1.SchemeGroupVersion.String() &&
		controllerRef.Kind == "NdbCluster" {
		return controllerRef.Name
	}

	return obj.GetLabels()[constants.ClusterLabel]
}

// extractAndEnqueueNdbCluster extracts the key of the NdbCluster that owns
// the given object and adds it to the workqueue for reconciliation.
func (c *Controller) extractAndEnqueueNdbCluster(obj metav1.Object, resource string, event string) {
	ndbClusterName := getOwningNdbClusterName(obj)
	if exists, _ := c.ndbClusterExists(obj.GetNamespace(), ndbClusterName); !exists {
		// Some error occurred during Get or the object doesn't exist
		// Skip enqueuing the object for reconciliation
		return
	}
	key := getNamespacedName2(obj.GetNamespace(), ndbClusterName)
	objName := getNamespacedName2(obj.GetNamespace(), obj.GetName())
	klog.Infof("NdbCluster resource %q is re-queued for further reconciliation as the %s %q owned by the NdbCluster is %s", key, resource, objName, event)
	c.workqueue.Add(key)
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until ctx is
// cancelled, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(ctx context.Context, threadiness int) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	klog.Info("Starting Ndb controller")

	// Wait for the caches to be synced before starting workers
	if ok := cache.WaitForNamedCacheSync(
		controllerName, ctx.Done(), c.informerSyncedMethods...); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	// Surface legacy exposure immediately instead of waiting for a later edit.
	c.recordManagementAccessWarningsForExistingClusters()
	// Launch worker go routines to process Ndb resources
	for i := 0; i < threadiness; i++ {
		go func() {
			// The workers continue processing work items
			// available in the work queue until they are shutdown
			for c.processNextWorkItem(ctx) {
			}
		}()
	}

	klog.Info("Started workers")
	<-ctx.Done()
	klog.Info("Shutting down workers")

	return nil
}

// processNextWorkItem reads a single work item off the
// workqueue and processes it, by calling the syncHandler.
func (c *Controller) processNextWorkItem(ctx context.Context) (continueProcessing bool) {
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
	sr := c.syncHandler(ctx, key)
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

	return true
}

func (c *Controller) newSyncContext(ndb *v1.NdbCluster) *SyncContext {
	return &SyncContext{
		mgmdController:           c.mgmdController,
		ndbmtdController:         c.ndbmtdController,
		mysqldController:         c.mysqldController,
		configMapController:      c.configMapController,
		serviceController:        c.serviceController,
		serviceaccountController: c.serviceAccountController,
		pdbController:            c.pdbController,
		networkPolicyController:  c.networkPolicyController,
		ndb:                      ndb,
		kubernetesClient:         c.kubernetesClient,
		ndbClient:                c.ndbClient,
		ndbsLister:               c.ndbsLister,
		podLister:                c.podLister,
		pvcLister:                c.pvcLister,
		serviceLister:            c.serviceLister,
		recorder:                 c.recorder,
	}
}

// syncHandler is the main reconciliation function
// driving cluster towards desired configuration
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
//  1. ensure all resources are correctly created
//  2. ensure cluster is fully up and running and not in a degraded state
//     before rolling out any changes
//  3. drive cluster components towards the configuration previously
//     written to the configuration file
//  4. only after complete cluster is aligned with configuration file
//     new changes from Ndb CRD are written to a new version of the config file
//  5. update status of the CRD
func (c *Controller) syncHandler(ctx context.Context, key string) (result syncResult) {

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

	// Create a syncContext with a DeepCopied NdbCluster resource
	// to prevent the sync method from accidentally mutating the
	// cache object.
	nc := ndbOrg.DeepCopy()
	syncContext := c.newSyncContext(nc)

	// Run sync.
	if result = syncContext.sync(ctx); result.getError() != nil {
		// The sync step returned an error - no need to update status yet
		return result
	}

	// Update the status of the NdbCluster resource
	statusUpdated, err := syncContext.updateNdbClusterStatus(ctx)
	if err != nil {
		// Status needs to be updated, but it failed.
		// Do not record events yet.
		// TODO: Ensure that the sync doesn't get stuck
		return result
	}

	// No error returned by updateNdbClusterStatus.
	// Check status and record events if necessary.
	if nc.Status.ProcessedGeneration == nc.Generation {
		// The latest generation of the NdbCluster has been processed
		// and the MySQL Cluster is up-to-date.
		if statusUpdated {
			// The status was updated in this loop implying that this
			// sync loop marks the end of the reconciliation of
			// MySQL Cluster configuration with the latest spec.
			// Record a SyncSuccess event to notify the same.
			syncContext.recorder.Eventf(nc, nil,
				corev1.EventTypeNormal, ReasonSyncSuccess, ActionSynced, MessageSyncSuccess)
		} else {
			// No status update this loop => the MySQL Cluster
			// was already in sync with the latest NdbCluster spec.
			// Record an InSync event
			syncContext.recorder.Eventf(nc, nil,
				corev1.EventTypeNormal, ReasonInSync, ActionNone, MessageInSync)
		}
	}

	return result
}

// managementNetworkPolicyAnnotationChanged detects metadata-only changes that
// do not increment generation.
func managementNetworkPolicyAnnotationChanged(oldNdb *v1.NdbCluster, newNdb *v1.NdbCluster) bool {
	oldValue, oldExists := oldNdb.GetAnnotations()[constants.ManagementNetworkPolicyAnnotation]
	newValue, newExists := newNdb.GetAnnotations()[constants.ManagementNetworkPolicyAnnotation]
	return oldExists != newExists || oldValue != newValue
}

// recordManagementAccessWarningsForExistingClusters audits legacy exceptions present at startup.
func (c *Controller) recordManagementAccessWarningsForExistingClusters() {
	ndbs, err := c.ndbsLister.List(labels.Everything())
	if err != nil {
		klog.Warningf("Failed to list NdbClusters for management access warning audit: %v", err)
		return
	}

	for _, ndb := range ndbs {
		c.recordManagementAccessWarnings(ndb)
	}
}

// recordManagementAccessWarnings reports each retained legacy exception.
func (c *Controller) recordManagementAccessWarnings(ndb *v1.NdbCluster) {
	if ndb.ManagementLoadBalancerEnabled() {
		klog.Warningf("NdbCluster resource %q uses deprecated spec.managementNode.enableLoadBalancer",
			getNamespacedName(ndb))
		c.recorder.Eventf(ndb, nil,
			corev1.EventTypeWarning, ReasonManagementLoadBalancerDeprecated, ActionNone,
			MessageManagementLoadBalancerDeprecated)
	}

	if !ndb.ManagementNetworkPolicyExpected() {
		klog.Warningf("NdbCluster resource %q explicitly disables the management node NetworkPolicy",
			getNamespacedName(ndb))
		c.recorder.Eventf(ndb, nil,
			corev1.EventTypeWarning, ReasonManagementNetworkPolicyDisabled, ActionNone,
			MessageManagementNetworkPolicyDisabled, constants.ManagementNetworkPolicyAnnotation)
	}
}
