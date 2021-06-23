// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	eventsv1 "k8s.io/api/events/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	appslisters "k8s.io/client-go/listers/apps/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	ndbclientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
	ndbscheme "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned/scheme"
	ndbinformers "github.com/mysql/ndb-operator/pkg/generated/informers/externalversions/ndbcontroller/v1alpha1"
	ndblisters "github.com/mysql/ndb-operator/pkg/generated/listers/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/helpers"
	"github.com/mysql/ndb-operator/pkg/mgmapi"
	"github.com/mysql/ndb-operator/pkg/resources"
)

const controllerName = "ndb-controller"

const (
	// ReasonResourceExists is the reason used for an Event when the
	// operator fails to sync the Ndb object with MySQL Cluster due to
	// some resource of the same name already existing but not owned by
	// the Ndb object.
	ReasonResourceExists = "ResourceExists"
	// ReasonSyncSuccess is the reason used for an Event when
	// the MySQL Cluster is successfully synced with the Ndb object.
	ReasonSyncSuccess = "SyncSuccess"
	// ReasonInSync is the reason used for an Event when the MySQL Cluster
	// is already in sync with the spec of the Ndb object.
	ReasonInSync = "InSync"

	// ActionNone is the action used for an Event when the operator does nothing.
	ActionNone = "None"
	// ActionSynced is the action used for an Event when the operator
	// makes changes to the MySQL Cluster and successfully syncs it with
	// the Ndb object.
	ActionSynced = "Synced"

	// MessageResourceExists is the message used for an Event when the
	// operator fails to sync the Ndb object with MySQL Cluster due to
	// some resource of the same name already existing but not owned by
	// the Ndb object.
	MessageResourceExists = "Resource %q already exists and is not managed by Ndb"
	// MessageSyncSuccess is the message used for an Event when
	// the MySQL Cluster is successfully synced with the Ndb object.
	MessageSyncSuccess = "MySQL Cluster was successfully synced up to match the spec"
	// MessageInSync is the message used for an Event when the MySQL Cluster
	// is already in sync with the spec of the Ndb object.
	MessageInSync = "MySQL Cluster is in sync with the Ndb object"
)

// ControllerContext summarizes the context in which it is running
type ControllerContext struct {
	// kubeClientset is the standard kubernetes clientset
	kubeClientset kubernetes.Interface
	ndbClientset  ndbclientset.Interface

	// runningInsideK8s is set to true if the operator is running inside a K8s cluster.
	runningInsideK8s bool
}

// SyncContext stores all information collected in/for a single run of syncHandler
type SyncContext struct {
	resourceContext *resources.ResourceContext

	dataNodeSfSet    *appsv1.StatefulSet
	mysqldDeployment *appsv1.Deployment

	ManagementServerPort int32
	ManagementServerIP   string

	clusterState *mgmapi.ClusterStatus

	ndb    *v1alpha1.Ndb
	nsName string

	// controller handling creation and changes of resources
	mysqldController    DeploymentControlInterface
	mgmdController      StatefulSetControlInterface
	ndbdController      StatefulSetControlInterface
	configMapController ConfigMapControlInterface

	controllerContext *ControllerContext
	ndbsLister        ndblisters.NdbLister

	// recorder is an event recorder for recording Event resources to the Kubernetes API.
	recorder events.EventRecorder

	// resource map stores the name of the resources already created and if they were created
	resourceMap *map[string]bool
}

// Controller is the main controller implementation for Ndb resources
type Controller struct {
	controllerContext *ControllerContext

	statefulSetLister       appslisters.StatefulSetLister
	statefulSetListerSynced cache.InformerSynced

	ndbsLister ndblisters.NdbLister
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

// NewController returns a new Ndb controller
func NewController(
	controllerContext *ControllerContext,
	statefulSetInformer appsinformers.StatefulSetInformer,
	deploymentInformer appsinformers.DeploymentInformer,
	serviceInformer coreinformers.ServiceInformer,
	podInformer coreinformers.PodInformer,
	configMapInformer coreinformers.ConfigMapInformer,
	ndbInformer ndbinformers.NdbInformer) *Controller {

	// Add ndb-controller types to the default Kubernetes Scheme
	// so Events can be logged for ndb-controller types.
	utilruntime.Must(ndbscheme.AddToScheme(scheme.Scheme))
	// Create event broadcaster
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := events.NewBroadcaster(
		&events.EventSinkImpl{
			Interface: controllerContext.kubeClientset.EventsV1(),
		})
	eventBroadcaster.StartRecordingToSink(make(chan struct{}))
	// setup additional logging for events
	eventBroadcaster.StartEventWatcher(func(event runtime.Object) {
		e := event.(*eventsv1.Event)
		klog.Infof("Event(%#v): type: '%s' action: '%s' reason: '%s' %s",
			e.Regarding, e.Type, e.Action, e.Reason, e.Note)
	})
	// create a new recorder to send events to the broadcaster
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, controllerName)

	controller := &Controller{
		controllerContext:       controllerContext,
		ndbsLister:              ndbInformer.Lister(),
		ndbsSynced:              ndbInformer.Informer().HasSynced,
		statefulSetLister:       statefulSetInformer.Lister(),
		statefulSetListerSynced: statefulSetInformer.Informer().HasSynced,
		deploymentLister:        deploymentInformer.Lister(),
		deploymentListerSynced:  deploymentInformer.Informer().HasSynced,
		serviceLister:           serviceInformer.Lister(),
		serviceListerSynced:     serviceInformer.Informer().HasSynced,
		podLister:               podInformer.Lister(),
		podListerSynced:         podInformer.Informer().HasSynced,
		configMapController:     NewConfigMapControl(controllerContext.kubeClientset, configMapInformer),
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
				klog.Infof("Difference in spec: %d : %d", oldNdb.Spec.NodeCount, newNdb.Spec.NodeCount)
			} else if !equality.Semantic.DeepEqual(oldNdb.Status, newNdb.Status) {
				klog.Infof("Difference in status")
			} else {
				klog.Infof("No difference in spec and status")
				//diff.ObjectGoPrintSideBySide(oldNdb, newNdb))
			}

			controller.enqueueNdb(newNdb)
		},
		DeleteFunc: func(obj interface{}) {
			var ndb *v1alpha1.Ndb
			switch objType := obj.(type) {
			case *v1alpha1.Ndb:
				ndb = objType
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
			pod := obj.(*corev1.Pod)
			ls := labels.Set(pod.Labels)
			if !ls.Has(constants.ClusterLabel) {
				return
			}
			//s, _ := json.MarshalIndent(pod.Status, "", "  ")
			//klog.Infof("%s", string(s))
			klog.Infof("pod new %s: phase= %s, ip=%s", pod.Name, pod.Status.Phase, pod.Status.PodIP)
		},
		UpdateFunc: func(old, new interface{}) {
			newPod := new.(*corev1.Pod)
			ls := labels.Set(newPod.Labels)
			if !ls.Has(constants.ClusterLabel) {
				return
			}

			//oldPod := old.(*corev1.Pod)
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
		c.deploymentListerSynced,
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

func (sc *SyncContext) kubeClientset() kubernetes.Interface {
	return sc.controllerContext.kubeClientset
}

func (sc *SyncContext) ndbClientset() ndbclientset.Interface {
	return sc.controllerContext.ndbClientset
}

func (sc *SyncContext) updateClusterLabels() error {

	ndb := sc.ndb.DeepCopy()
	lbls := sc.ndb.GetLabels()

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		ndb.Labels = labels.Merge(ndb.Labels, lbls)
		_, updateErr :=
			sc.ndbClientset().MysqlV1alpha1().Ndbs(ndb.Namespace).Update(context.TODO(), ndb, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}

		key := fmt.Sprintf("%s/%s", ndb.GetNamespace(), ndb.GetName())
		klog.V(4).Infof("Conflict updating Cluster labels. Getting updated Cluster %s from cache...", key)

		updated, err := sc.ndbClientset().MysqlV1alpha1().Ndbs(ndb.Namespace).Get(context.TODO(), ndb.Name, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("Error getting updated Cluster %q: %v", key, err)
			return err
		}

		// Copy the Cluster so we don't mutate the cache.
		ndb = updated.DeepCopy()
		return updateErr
	})
}

// ensureService creates a services if it doesn't exist
// returns
//    service existing or created
//    true if services was created
//    error if any such occurred
func (sc *SyncContext) ensureService(port int32, selector string, createLoadBalancer bool) (*corev1.Service, bool, error) {

	serviceName := sc.ndb.GetServiceName(selector)
	if createLoadBalancer {
		serviceName += "-ext"
	}

	svc, err := sc.kubeClientset().CoreV1().Services(sc.ndb.Namespace).Get(context.TODO(), serviceName, metav1.GetOptions{})

	if err == nil {
		return svc, true, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, false, err
	}

	// Service not found - create it
	klog.Infof("Creating a new Service %s for cluster %q", serviceName,
		types.NamespacedName{Namespace: sc.ndb.Namespace, Name: sc.ndb.Name})
	svc = resources.NewService(sc.ndb, port, selector, createLoadBalancer)
	svc, err = sc.kubeClientset().CoreV1().Services(sc.ndb.Namespace).Create(context.TODO(), svc, metav1.CreateOptions{})
	if err != nil {
		return nil, false, err
	}
	return svc, false, err
}

// ensure services creates services if they don't exist
// returns
//    array with services created
//    false if any services were created
//    error if any such occurred
//
// at this stage of the functionality we can still create resources even if
// the config is invalid as services are only based on Ndb CRD name
// which is obviously immutable for each individual Ndb CRD
func (sc *SyncContext) ensureServices() (*[]*corev1.Service, bool, error) {

	var svcs []*corev1.Service
	retExisted := true

	// create a headless service for management nodes
	svc, existed, err := sc.ensureService(1186, sc.mgmdController.GetTypeName(), false)
	if err != nil {
		return nil, false, err
	}
	retExisted = retExisted && existed
	svcs = append(svcs, svc)

	// create a load balancer service for management servers
	svc, existed, err = sc.ensureService(1186, sc.mgmdController.GetTypeName(), true)
	if err != nil {
		return nil, false, err
	}
	retExisted = retExisted && existed
	svcs = append(svcs, svc)
	// store the management IP and port
	sc.ManagementServerIP, sc.ManagementServerPort = helpers.GetServiceAddressAndPort(svc)

	// create a headless service for data nodes
	svc, existed, err = sc.ensureService(1186, sc.ndbdController.GetTypeName(), false)
	if err != nil {
		return nil, false, err
	}
	retExisted = retExisted && existed
	svcs = append(svcs, svc)

	// create a loadbalancer for MySQL Servers in the deployment
	svc, existed, err = sc.ensureService(3306, sc.mysqldController.GetTypeName(), true)
	if err != nil {
		return nil, false, err
	}
	retExisted = retExisted && existed
	svcs = append(svcs, svc)

	return &svcs, retExisted, nil
}

// ensureManagementServerStatefulSet creates the stateful set for management servers if it doesn't exist
// returns
//    new or existing statefulset
//    reports true if it existed
//    or returns an error if something went wrong
func (sc *SyncContext) ensureManagementServerStatefulSet() (*appsv1.StatefulSet, bool, error) {

	sfset, existed, err := sc.mgmdController.EnsureStatefulSet(sc)
	if err != nil {
		return nil, existed, err
	}

	if sfset == nil {
		// didn't exist and wasn't created wither
		return nil, existed, nil
	}

	// If the StatefulSet is not controlled by this Ndb resource, we should log
	// a warning to the event recorder and return error msg.
	if !metav1.IsControlledBy(sfset, sc.ndb) {
		msg := fmt.Sprintf(MessageResourceExists, sfset.Name)
		sc.recorder.Eventf(sc.ndb, nil,
			corev1.EventTypeWarning, ReasonResourceExists, ActionNone, MessageResourceExists)
		return nil, existed, fmt.Errorf(msg)
	}

	return sfset, existed, err
}

// ensureDataNodeStatefulSet creates the stateful set for data node if it doesn't exist
// returns
//    new or existing statefulset
//    reports true if it existed
//    or returns an error if something went wrong
func (sc *SyncContext) ensureDataNodeStatefulSet() (*appsv1.StatefulSet, bool, error) {

	sfset, existed, err := sc.ndbdController.EnsureStatefulSet(sc)
	if err != nil {
		return nil, existed, err
	}

	if sfset == nil {
		// didn't exist and wasn't created wither
		return nil, existed, nil
	}

	// If the StatefulSet is not controlled by this Ndb resource, we should log
	// a warning to the event recorder and return error msg.
	if !metav1.IsControlledBy(sfset, sc.ndb) {
		msg := fmt.Sprintf(MessageResourceExists, sfset.Name)
		sc.recorder.Eventf(sc.ndb, nil,
			corev1.EventTypeWarning, ReasonResourceExists, ActionNone, msg)
		return nil, existed, fmt.Errorf(msg)
	}

	return sfset, existed, err
}

// ensureMySQLServerDeployment creates a deployment of MySQL Servers if doesn't exist
// returns
//    true if the deployment exists
//    or returns an error if something went wrong
func (sc *SyncContext) ensureMySQLServerDeployment() (*appsv1.Deployment, bool, error) {

	deployment, existed, err := sc.mysqldController.EnsureDeployment(context.TODO(), sc)
	if err != nil {
		// Failed to ensure the deployment
		return deployment, existed, err
	}

	if deployment == nil {
		// didn't exist and wasn't created wither
		return nil, existed, nil
	}

	// If the deployment is not controlled by Ndb resource,
	// log a warning to the event recorder and return the error message.
	if !metav1.IsControlledBy(deployment, sc.ndb) {
		msg := fmt.Sprintf(MessageResourceExists, deployment.Name)
		sc.recorder.Eventf(sc.ndb, nil,
			corev1.EventTypeWarning, ReasonResourceExists, ActionNone, msg)
		return deployment, existed, fmt.Errorf(msg)
	}

	return deployment, existed, nil
}

// ensurePodDisruptionBudget creates a PDB if it doesn't exist
// returns
//    new or existing PDB
//    reports true if it existed
//    or returns an error if something went wrong
//
// PDB can't be created if Ndb is invalid; it depends on data node count
func (sc *SyncContext) ensurePodDisruptionBudget() (*policyv1beta1.PodDisruptionBudget, bool, error) {

	// Check if ndbd pod disruption budget is present
	nodeType := sc.ndbdController.GetTypeName()
	pdbs := sc.kubeClientset().PolicyV1beta1().PodDisruptionBudgets(sc.ndb.Namespace)
	pdb, err := pdbs.Get(context.TODO(), sc.ndb.GetPodDisruptionBudgetName(nodeType), metav1.GetOptions{})
	if err == nil {
		return pdb, true, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, false, err
	}

	klog.Infof("Creating a new PodDisruptionBudget for Data Nodes of Cluster %q",
		types.NamespacedName{Namespace: sc.ndb.Namespace, Name: sc.ndb.Name})
	pdb = resources.NewPodDisruptionBudget(sc.ndb, nodeType)
	pdb, err = pdbs.Create(context.TODO(), pdb, metav1.CreateOptions{})

	if err != nil {
		return nil, false, err
	}

	return pdb, false, err
}

// ensureDataNodeConfigVersion checks if all the data nodes have the desired configuration.
// If not, it safely restarts them without affecting the availability of MySQL Cluster.
func (sc *SyncContext) ensureDataNodeConfigVersion() syncResult {

	wantedGeneration := sc.resourceContext.ConfigGeneration

	// Every data node is assigned a unique replica number
	// (0 to N-1, where N is the redundancyLevel), within their nodegroup.
	// All data nodes with the same replica number are stopped and
	// restarted during a single reconciliation run. At any given time,
	// only one data node per group will be affected by this maneuver,
	// thus ensuring availability of the MySQL Cluster.
	ct := mgmapi.CreateClusterTopologyByReplicaFromClusterStatus(sc.clusterState)
	if ct == nil {
		err := fmt.Errorf("internal error: could not extract topology from cluster status")
		return errorWhileProcssing(err)
	}

	redundancyLevel := ct.GetNumberOfReplicas()
	mgmClient, err := sc.connectToManagementServer()
	if err != nil {
		return errorWhileProcssing(err)
	}
	defer mgmClient.Disconnect()

	for replica := 0; replica < redundancyLevel; replica++ {

		var restartIDs []int
		nodeIDs := ct.GetNodeIDsFromReplica(replica)
		for _, nodeID := range *nodeIDs {
			nodeConfigGeneration, err := mgmClient.GetConfigVersionFromNode(nodeID)
			if err != nil {
				return errorWhileProcssing(err)
			}

			if wantedGeneration != nodeConfigGeneration {
				// data node runs with an old versioned config
				restartIDs = append(restartIDs, nodeID)
			}
		}

		if len(restartIDs) > 0 {
			// restart all the identified data nodes
			s, _ := json.Marshal(restartIDs)
			klog.Infof("Identified %d nodes with wrong version in replica %d: %s",
				len(restartIDs), replica, s)

			err := mgmClient.StopNodes(restartIDs)
			if err != nil {
				klog.Infof("Error restarting replica %d nodes %s", replica, s)
				return errorWhileProcssing(err)
			}

			// The data nodes have started to stop.
			// Exit here and allow them to be restarted by the statefulset controllers.
			// Continue syncing once they are up, in a later reconciliation loop.
			return finishProcessing()
		}

		klog.Infof("All datanodes nodes in replica %d have desired config version %d", replica, wantedGeneration)
	}

	// All data nodes have the desired config version. Continue with rest of the sync process.
	return continueProcessing()
}

// connectToManagementServer connects to a management server and returns the mgmapi.MgmClient
// An optional managementNodeId can be passed to force the method to connect to the mgmd with the id.
func (sc *SyncContext) connectToManagementServer(managementNodeId ...int) (mgmapi.MgmClient, error) {

	if len(managementNodeId) > 1 {
		// connectToManagementServer usage error
		panic("nodeId can take in only one optional management node id to connect to")
	}

	// By default, connect to the mgmd with nodeId 1
	nodeId := 1
	if len(managementNodeId) == 1 {
		nodeId = managementNodeId[0]
	}

	// Deduce the connectstring
	var connectstring string
	if sc.controllerContext.runningInsideK8s {
		// Operator is running inside a K8s pod.
		// Directly connect to the desired management server's pod using its IP
		podName := fmt.Sprintf("%s-%d", sc.ndb.GetServiceName("mgmd"), nodeId-1)
		pod, err := sc.kubeClientset().CoreV1().Pods(sc.ndb.Namespace).Get(context.TODO(), podName, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("Management server with node id '%d' and pod '%s/%s' not found",
				nodeId, sc.ndb.Namespace, podName)
			return nil, err
		}
		connectstring = fmt.Sprintf("%s:%d", pod.Status.PodIP, 1186)
		klog.Infof("Using pod %s/%s for management server with node id %d", sc.ndb.Namespace, podName, nodeId)
	} else {
		// Operator is running from outside K8s.
		// Connect to the Management server via load balancer.
		// The MgmClient will retry connecting via the load balancer
		// until a connection is established to the desired node.
		connectstring = fmt.Sprintf("%s:%d", sc.ManagementServerIP, sc.ManagementServerPort)
	}

	mgmClient, err := mgmapi.NewMgmClient(connectstring, nodeId)
	if err != nil {
		klog.Errorf(
			"Failed to connect to the Management Server with desired node id %d : %s",
			nodeId, err)
		return nil, err
	}

	return mgmClient, nil
}

func (sc *SyncContext) ensureManagementServerConfigVersion() syncResult {
	wantedGeneration := sc.resourceContext.ConfigGeneration
	klog.Infof("Ensuring Management Server has correct config version %d", wantedGeneration)

	// Management servers have the first one/two node ids
	for nodeID := 1; nodeID <= (int)(sc.ndb.GetManagementNodeCount()); nodeID++ {
		mgmClient, err := sc.connectToManagementServer(nodeID)
		if err != nil {
			return errorWhileProcssing(err)
		}

		version, err := mgmClient.GetConfigVersion()
		if err != nil {
			return errorWhileProcssing(err)
		}

		if version == wantedGeneration {
			klog.Infof("Management server with node id %d has desired version %d",
				nodeID, version)

			// This Management Server already has the desired config version.
			mgmClient.Disconnect()
			continue
		}

		klog.Infof("Management server with node id %d has different version %d than desired %d",
			nodeID, version, wantedGeneration)

		// The Management Server does not have the desired config version.
		// Stop it and let the statefulset controller start the server with the correct, recent config.
		nodeIDs := []int{nodeID}
		err = mgmClient.StopNodes(nodeIDs)
		if err != nil {
			klog.Errorf("Error stopping management node %v", err)
		}
		mgmClient.Disconnect()

		// Management Server has been stopped. Trigger only one restart
		// at a time and handle the rest in later reconciliations.
		return finishProcessing()
	}

	// All Management Servers have the latest config.
	// Continue further processing and sync.
	return continueProcessing()
}

// checkPodStatus returns false if any container in pod is not ready
// TODO - should also look out for hanging or weird states
func (sc *SyncContext) checkPodStatus() (bool, error) {

	klog.Infof("check Pod status in namespace %s", sc.ndb.Namespace)

	listOptions := metav1.ListOptions{
		LabelSelector: labels.Set(sc.ndb.GetLabels()).String(),
		Limit:         256,
	}
	pods, err := sc.kubeClientset().CoreV1().Pods(sc.ndb.Namespace).List(context.TODO(), listOptions)
	if err != nil {
		return false, apierrors.NewNotFound(v1alpha1.Resource("Pod"), listOptions.LabelSelector)
	}

	for _, pod := range pods.Items {
		status := pod.Status
		statuses := status.ContainerStatuses
		for _, status := range statuses {
			if status.Name != "ndbd" && status.Name != "mgmd" {
				continue
			}
			if !status.Ready {
				return false, nil
			}
		}
	}

	return true, nil
}

// Ensure that the required labels are set on the cluster.
//
// Labels are placed on the ndb resource based on the name.
func (sc *SyncContext) ensureClusterLabel() (*labels.Set, bool, error) {

	sel4ndb := labels.SelectorFromSet(sc.ndb.GetLabels())
	set := labels.Set(sc.ndb.Labels)
	if sel4ndb.Matches(set) {
		return &set, true, nil
	}

	klog.Infof("Setting labels on cluster %s", sel4ndb.String())
	err := sc.updateClusterLabels()
	if err != nil {
		return nil, false, err
	}

	return &set, false, nil

}

func (sc *SyncContext) getClusterState() error {

	mgmClient, err := sc.connectToManagementServer()
	if err != nil {
		return err
	}
	defer mgmClient.Disconnect()

	// check if management nodes report a degraded cluster state
	cs, err := mgmClient.GetStatus()
	if err != nil {
		klog.Errorf("Error getting cluster status from management server: %s", err)
		return err
	}

	sc.clusterState = cs

	return nil
}

// checkClusterState checks the cluster state and whether its in a managable state
// e.g. - we don't want to touch it with rolling out new versions when not all data nodes are up
func (sc *SyncContext) checkClusterState() syncResult {

	err := sc.getClusterState()
	if err != nil {
		return errorWhileProcssing(err)
	}

	// cluster is okay if all active node groups have all data nodes up

	// during scaling ndb CRD will have more nodes configured
	// this will already be written to config file in a first sync step
	// but statefulsets will adopt no of replicas as last step before node group is created
	nodeGroupsUp, scalingNodes := sc.clusterState.NumberNodegroupsFullyUp(int(sc.resourceContext.RedundancyLevel))
	numberOfDataNodes := nodeGroupsUp * int(sc.resourceContext.RedundancyLevel)

	if int(*sc.dataNodeSfSet.Spec.Replicas) == numberOfDataNodes {
		// all nodes that should be up have all nodes running
		return continueProcessing()
	}
	if int(*sc.dataNodeSfSet.Spec.Replicas) == numberOfDataNodes+scalingNodes {
		// all nodes that should be up have all nodes running
		return continueProcessing()
	}
	return finishProcessing()
}

// allResourcesExisted returns falls if any resource map is false (resource was created)
func (sc *SyncContext) allResourcesExisted() bool {

	retExisted := true
	for res, existed := range *sc.resourceMap {
		if existed {
			klog.Infof("Resource %s: existed", res)
		} else {
			klog.Infof("Resource %s: created", res)
			retExisted = false
		}
	}
	return retExisted
}

// ensureAllResources creates all resources if they do not exist
// the SyncContext struct will be filled with resource objects newly created or fetched
// it returns
//   false if any resource did not exist
//   an error if such occurs during processing
//
// Resource creation as all other steps in syncHandler need to be idempotent.
//
// However, creation of multiple resources is obviously not one large atomic operation.
// ndb.Status updates based on resources created can't be done atomically either.
// The creation processes could any time be disrupted by crashes, termination or (temp) errors.
//
// The goal is to create a consistent setup.
// The cluster configuration (file) in config map needs to match e.g. the no of replicas
// the in stateful sets even though ndb.Spec could change between config map and sfset creation.
//
// In order to solve these issues the configuration store in the config file is considered
// the source of the truth during the entire creation process. Only after all resources once
// successfully created changes to the ndb.Spec will be considered by the syncHandler.
func (sc *SyncContext) ensureAllResources() (bool, error) {

	// create labels on ndb resource
	// TODO - not sure if we need a cluster level label on the CRD
	//      causes an update event looping us in here again
	var err error
	if _, (*sc.resourceMap)["labels"], err = sc.ensureClusterLabel(); err != nil {
		return false, err
	}

	// create services for management server and data node statefulsets
	// with respect to idempotency and atomicy service creation is always safe as it
	// only uses the immutable CRD name
	// service needs to be created and present when creating stateful sets
	if _, (*sc.resourceMap)["services"], err = sc.ensureServices(); err != nil {
		return false, err
	}

	// create pod disruption budgets
	if _, (*sc.resourceMap)["poddisruptionservice"], err = sc.ensurePodDisruptionBudget(); err != nil {
		return false, err
	}

	// create config map if not exist
	var cm *corev1.ConfigMap
	if cm, (*sc.resourceMap)["configmap"], err = sc.configMapController.EnsureConfigMap(sc); err != nil {
		return false, err
	}

	// get config string from config
	// this and following step could be avoided since we in most cases just created the config map
	// however, resource creation (happens once) or later modification as such is probably the unlikely case
	// much more likely in all cases is that the config map already existed
	if cm != nil {
		// resource was created or existed
		configString, err := sc.configMapController.ExtractConfig(cm)
		if err != nil {
			// less likely to happen as the config value cannot go missing at this point
			return false, err
		}

		sc.resourceContext, err = resources.NewResourceContextFromConfiguration(configString)
		if err != nil {
			// less likely to happen the only possible error is a config
			// parse error, and the configString was generated by the operator
			return false, err
		}
	}

	// even if config is invalid following functions should still test if
	// resources already exist because we can then continue

	// create the management stateful set if it doesn't exist
	if _, (*sc.resourceMap)["mgmstatefulset"], err = sc.ensureManagementServerStatefulSet(); err != nil {
		return false, err
	}

	// create the data node stateful set if it doesn't exist
	if sc.dataNodeSfSet, (*sc.resourceMap)["datanodestatefulset"], err = sc.ensureDataNodeStatefulSet(); err != nil {
		return false, err
	}

	// create the mysql deployment if it doesn't exist
	if sc.mysqldDeployment, (*sc.resourceMap)["mysqldeployment"], err = sc.ensureMySQLServerDeployment(); err != nil {
		return false, err
	}

	return sc.allResourcesExisted(), nil
}

func (c *Controller) newSyncContext(ndb *v1alpha1.Ndb) *SyncContext {

	//TODO: should probably create controller earlier
	if c.ndbdController == nil {
		ndbdSfSet := resources.NewNdbdStatefulSet(ndb)
		c.ndbdController =
			&realStatefulSetControl{
				client:            c.controllerContext.kubeClientset,
				statefulSetLister: c.statefulSetLister,
				statefulSetType:   ndbdSfSet}
	}
	// create the management stateful set if it doesn't exist
	if c.mgmdController == nil {
		mgmdSfSet := resources.NewMgmdStatefulSet(ndb)
		c.mgmdController =
			&realStatefulSetControl{
				client:            c.controllerContext.kubeClientset,
				statefulSetLister: c.statefulSetLister,
				statefulSetType:   mgmdSfSet}
	}
	if c.mysqldController == nil {
		c.mysqldController = &mysqlDeploymentController{
			client:                c.controllerContext.kubeClientset,
			deploymentLister:      c.deploymentLister,
			mysqlServerDeployment: resources.NewMySQLServerDeployment(ndb),
		}
	}

	resourceMap := make(map[string]bool)
	namespaceName := types.NamespacedName{Namespace: ndb.GetNamespace(), Name: ndb.GetName()}.String()
	return &SyncContext{
		mgmdController:      c.mgmdController,
		ndbdController:      c.ndbdController,
		mysqldController:    c.mysqldController,
		configMapController: c.configMapController,
		ndb:                 ndb,
		resourceMap:         &resourceMap,
		controllerContext:   c.controllerContext,
		ndbsLister:          c.ndbsLister,
		recorder:            c.recorder,
		nsName:              namespaceName,
	}
}

// syncHandler is the main reconcilliation function
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
func (c *Controller) syncHandler(key string) error {

	klog.Infof("Sync handler: %s", key)

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

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

	syncContext := c.newSyncContext(ndb)

	return syncContext.sync()
}

func (sc *SyncContext) sync() error {

	// create all resources necessary to start and run cluster
	existed, err := sc.ensureAllResources()
	if err != nil {
		return err
	}

	// we do not take further action and exit here if any resources still had to be created
	// pods will need to time to start, etc.
	//
	// since no error occured all resources should have been successfully created
	// TODO: we should take note of it by updating the ndb.Status with a creationTimestamp
	// - then we can check that later and avoid checking if all resources existed
	// - could be also good to always check if individual resources were deleted
	//   but re-creating them could be tricky due to resource dependencies bringing us to chaotic state
	// - if individual resources are missing after creation that would be a major error for now
	if !existed {
		return nil
	}

	// at this point all resources were created already
	// some pods might still not be fully up and running
	// cluster potentially not (fully) started yet

	if ready, _ := sc.checkPodStatus(); !ready {
		// if not all pods are ready yet there is no sense in processing config changes
		klog.Infof("Cluster has not all pods ready - exit sync and return later")
		return nil
	}

	//
	// as of here actions will only be taken if cluster is in a good state
	//

	if sr := sc.checkClusterState(); sr.finished() {
		klog.Infof("Cluster is not reported to be fully running - exit sync and return here later")
		// TODO - introduce a re-schedule event
		return sr.getError() // return error if any
	}

	// sync handler does not accept new configurations from Ndb CRD
	// before previous configuration changes are not completed
	// start by aligning cluster to the configuration *in the config map* previously applied
	// only if everything is in line with that configuration
	// a new configuration from the Ndb CRD is accepted and written to the config map

	// First pass of MySQL Server reconciliation.
	// If any scale down was requested, it will be handled in this pass.
	// This is done separately to ensure that the MySQL Servers are shut
	// down before possibly reducing the number of API sections in config.
	if sr := sc.mysqldController.ReconcileDeployment(sc.ndb, sc.mysqldDeployment, sc.resourceContext, true); sr.finished() {
		return sr.getError()
	}

	// make sure management server(s) have the correct config version
	if sr := sc.ensureManagementServerConfigVersion(); sr.finished() {
		return sr.getError()
	}

	// make sure all data nodes have the correct config version
	// data nodes a restarted with respect to
	if sr := sc.ensureDataNodeConfigVersion(); sr.finished() {
		return sr.getError()
	}

	// If this number of the members on the Cluster does not equal the
	// current desired replicas on the StatefulSet, we should update the
	// StatefulSet resource.
	if sc.resourceContext.GetDataNodeCount() != uint32(*sc.dataNodeSfSet.Spec.Replicas) {
		klog.Infof("Updating %q: DataNodes=%d statefulSetReplicas=%d",
			sc.nsName, sc.ndb.Spec.NodeCount, *sc.dataNodeSfSet.Spec.Replicas)
		if sc.dataNodeSfSet, err = sc.ndbdController.Patch(sc.resourceContext, sc.ndb, sc.dataNodeSfSet); err != nil {
			// Requeue the item so we can attempt processing again later.
			// This could have been caused by a temporary network failure etc.
			return err
		}
	}

	// Second pass of MySQL Server reconciliation
	// Reconcile the rest of spec/config change in MySQL Server Deployment
	if sr := sc.mysqldController.ReconcileDeployment(sc.ndb, sc.mysqldDeployment, sc.resourceContext, false); sr.finished() {
		return sr.getError()
	}

	// At this point, the MySQL Cluster is in sync with the configuration in the config map.
	// The configuration in the config map has to be checked to see if it is still the
	// desired config specified in the Ndb object.
	klog.Infof("Config in config map config is \"%s\", generation: \"%d\"",
		sc.resourceContext.ConfigHash, sc.resourceContext.ConfigGeneration)

	// calculate the hash of the new config
	newConfigHash, err := sc.ndb.CalculateNewConfigHash()
	if err != nil {
		klog.Errorf("Error calculating hash of incoming Ndb resource.")
		return err
	}

	// Check if configuration in config map is still the desired from the Ndb CRD
	hasPendingConfigChanges := sc.resourceContext.ConfigHash != newConfigHash
	if hasPendingConfigChanges {
		// The Ndb object spec has changed - patch the config map
		klog.Infof("Config received is different from existing config map config. config map: \"%s\", new: \"%s\"",
			sc.resourceContext.ConfigHash, newConfigHash)
		_, err := sc.configMapController.PatchConfigMap(sc.ndb, sc.resourceContext)
		if err != nil {
			klog.Infof("Failed to patch config map: %s", err)
			return err
		}
	}

	// Update the status of the Ndb resource to reflect the state of any changes applied
	err = sc.updateNdbStatus(hasPendingConfigChanges)
	if err != nil {
		klog.Errorf("Updating status failed: %v", err)
		return err
	}

	klog.V(4).Infof("Returning from syncHandler")

	return nil
}

func (sc *SyncContext) updateNdbStatus(hasPendingConfigChanges bool) error {

	// we already received a deep copy here
	ndb := sc.ndb

	if hasPendingConfigChanges {
		// The loop received a new config change that has to be applied yet
		if ndb.Status.ProcessedGeneration+1 == ndb.ObjectMeta.Generation {
			// All the previous generations have been handled already
			// and the status has been updated.
			// Do not update status yet for the current change.
			return nil
		} else {
			// All the config changes except the one received in this
			// loop has been handled but the status is not updated yet.
			// Bump up the ProcessedGeneration to reflect this.
			ndb.Status.ProcessedGeneration = ndb.ObjectMeta.Generation - 1
		}
	} else {
		// No pending changes
		if ndb.Status.ProcessedGeneration == ndb.ObjectMeta.Generation {
			// Nothing happened in this loop. Skip updating status.
			// Record an InSync event and return
			sc.recorder.Eventf(sc.ndb, nil,
				corev1.EventTypeNormal, ReasonInSync, ActionNone, MessageInSync)
			return nil
		} else {
			// The last change was successfully applied.
			// Update status to reflect this
			ndb.Status.ProcessedGeneration = ndb.ObjectMeta.Generation
		}
	}

	// Set the time of this status update
	ndb.Status.LastUpdate = metav1.NewTime(time.Now())

	updateErr := wait.ExponentialBackoff(retry.DefaultBackoff, func() (ok bool, err error) {

		klog.Infof("Updating ndb cluster status: from process gen %d to %d",
			ndb.Status.ProcessedGeneration, ndb.ObjectMeta.Generation)

		_, err = sc.ndbClientset().MysqlV1alpha1().Ndbs(ndb.Namespace).UpdateStatus(context.TODO(), ndb, metav1.UpdateOptions{})
		if err == nil {
			return true, nil
		}
		if !errors.IsConflict(err) {
			return false, err
		}

		updated, err := sc.ndbClientset().MysqlV1alpha1().Ndbs(ndb.Namespace).Get(context.TODO(), ndb.Name, metav1.GetOptions{})
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

	// Record an SyncSuccess event as the MySQL Cluster specification has been
	// successfully synced with the spec of Ndb object and the status has been updated.
	sc.recorder.Eventf(sc.ndb, nil,
		corev1.EventTypeNormal, ReasonSyncSuccess, ActionSynced, MessageSyncSuccess)

	return nil
}

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
