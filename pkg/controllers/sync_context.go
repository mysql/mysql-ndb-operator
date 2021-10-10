// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"fmt"
	ndbclientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
	"github.com/mysql/ndb-operator/pkg/helpers"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"time"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	ndblisters "github.com/mysql/ndb-operator/pkg/generated/listers/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/mgmapi"
	"github.com/mysql/ndb-operator/pkg/resources"
)

// SyncContext stores all information collected in/for a single run of syncHandler
type SyncContext struct {
	resourceContext *resources.ResourceContext

	dataNodeSfSet    *appsv1.StatefulSet
	mysqldDeployment *appsv1.Deployment

	ManagementServerPort int32
	ManagementServerIP   string

	clusterState mgmapi.ClusterStatus

	ndb *v1alpha1.NdbCluster

	// controller handling creation and changes of resources
	mysqldController    DeploymentControlInterface
	mgmdController      StatefulSetControlInterface
	ndbdController      StatefulSetControlInterface
	configMapController ConfigMapControlInterface

	controllerContext *ControllerContext
	ndbsLister        ndblisters.NdbClusterLister
	podLister         listerscorev1.PodLister
	serviceLister     listerscorev1.ServiceLister

	// recorder is an event recorder for recording Event resources to the Kubernetes API.
	recorder events.EventRecorder
}

func (sc *SyncContext) kubeClientset() kubernetes.Interface {
	return sc.controllerContext.kubeClientset
}

func (sc *SyncContext) ndbClientset() ndbclientset.Interface {
	return sc.controllerContext.ndbClientset
}

// isOwnedByNdbCluster returns an error if the given
// object is not owned by the NdbCluster resource being synced.
func (sc *SyncContext) isOwnedByNdbCluster(object metav1.Object) error {
	if !metav1.IsControlledBy(object, sc.ndb) {
		// The object is not owned by the NdbCluster resource being synced.
		// Record a warning and return an error.
		err := fmt.Errorf(MessageResourceExists, getNamespacedName(object))
		sc.recorder.Eventf(sc.ndb, nil,
			corev1.EventTypeWarning, ReasonResourceExists, ActionNone, err.Error())
		return err
	}
	return nil
}

// ensureService creates a service if it doesn't exist yet
func (sc *SyncContext) ensureService(
	ctx context.Context, port int32,
	selector string, createLoadBalancer bool) (svc *corev1.Service, existed bool, err error) {

	serviceName := sc.ndb.GetServiceName(selector)
	if createLoadBalancer {
		serviceName += "-ext"
	}

	svc, err = sc.serviceLister.Services(sc.ndb.Namespace).Get(serviceName)

	if err == nil {
		// Service exists already
		if err = sc.isOwnedByNdbCluster(svc); err != nil {
			// But it is not owned by the NdbCluster resource
			return nil, false, err
		}

		// Service exists and is owned by the current NdbCluster
		return svc, true, nil
	}

	if !apierrors.IsNotFound(err) {
		return nil, false, err
	}

	// Service not found - create it
	klog.Infof("Creating a new Service %s for cluster %q", serviceName, getNamespacedName(sc.ndb))
	svc = resources.NewService(sc.ndb, port, selector, createLoadBalancer)
	svc, err = sc.kubeClientset().CoreV1().Services(sc.ndb.Namespace).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		// Create failed. Ignore AlreadyExists error as it might
		// have been caused due to a previous, outdated, cache read.
		return nil, false, err
	}
	return svc, false, err
}

// ensureServices creates all the necessary services for the
// MySQL Cluster if they do not exist yet.
// at this stage of the functionality we can still create resources even if
// the config is invalid as services are only based on Ndb CRD name
// which is obviously immutable for each individual Ndb CRD
func (sc *SyncContext) ensureServices(
	ctx context.Context) (svcs []*corev1.Service, allServicesExisted bool, err error) {

	// create a headless service for management nodes
	svc, existed, err := sc.ensureService(ctx, 1186, sc.mgmdController.GetTypeName(), false)
	if err != nil {
		return nil, false, err
	}
	allServicesExisted = existed
	svcs = append(svcs, svc)

	// create a load balancer service for management servers
	svc, existed, err = sc.ensureService(ctx, 1186, sc.mgmdController.GetTypeName(), true)
	if err != nil {
		return nil, false, err
	}
	allServicesExisted = allServicesExisted && existed
	svcs = append(svcs, svc)
	// store the management IP and port
	sc.ManagementServerIP, sc.ManagementServerPort = helpers.GetServiceAddressAndPort(svc)

	// create a headless service for data nodes
	svc, existed, err = sc.ensureService(ctx, 1186, sc.ndbdController.GetTypeName(), false)
	if err != nil {
		return nil, false, err
	}
	allServicesExisted = allServicesExisted && existed
	svcs = append(svcs, svc)

	// create a loadbalancer for MySQL Servers in the deployment
	svc, existed, err = sc.ensureService(ctx, 3306, sc.mysqldController.GetTypeName(), true)
	if err != nil {
		return nil, false, err
	}
	allServicesExisted = allServicesExisted && existed
	svcs = append(svcs, svc)

	return svcs, allServicesExisted, nil
}

// ensureManagementServerStatefulSet creates the StatefulSet for
// Management Server in the K8s Server if they don't exist yet.
func (sc *SyncContext) ensureManagementServerStatefulSet(
	ctx context.Context) (mgmdStatefulSet *appsv1.StatefulSet, existed bool, err error) {
	return sc.mgmdController.EnsureStatefulSet(ctx, sc)
}

// ensureDataNodeStatefulSet creates the StatefulSet for
// Data Nodes in the K8s Server if they don't exist yet.
func (sc *SyncContext) ensureDataNodeStatefulSet(
	ctx context.Context) (ndbdStatefulSet *appsv1.StatefulSet, existed bool, err error) {
	return sc.ndbdController.EnsureStatefulSet(ctx, sc)
}

// validateMySQLServerDeployment retrieves the MySQL Server deployment from K8s.
// If the deployment exists, it verifies if it is owned by the NdbCluster resource.
func (sc *SyncContext) validateMySQLServerDeployment() (*appsv1.Deployment, error) {
	return sc.mysqldController.GetDeployment(sc)
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
	redundancyLevel := sc.resourceContext.RedundancyLevel

	mgmClient, err := sc.connectToManagementServer()
	if err != nil {
		return errorWhileProcessing(err)
	}
	defer mgmClient.Disconnect()

	// For data node reconciliation, one data node per nodegroup is
	// chosen and are restarted together if they have an outdated
	// config version. Once a set of data nodes are restarted, any
	// further reconciliation is stopped, and is resumed only after
	// the restarted data nodes become ready. At any given time,
	// only one data node per group will be affected by this
	// maneuver thus ensuring MySQL Cluster's availability.
	nodesGroupedByNodegroups := sc.clusterState.GetNodesGroupedByNodegroup()
	if nodesGroupedByNodegroups == nil {
		err := fmt.Errorf("internal error: could not extract nodes and node groups from cluster status")
		return errorWhileProcessing(err)
	}

	// The node ids are sorted within the sub arrays and the array
	// itself is sorted based on the node groups. Every sub array
	// will have RedundancyLevel number of node ids. Pick up the
	// i'th node from every sub array during every iteration and
	// ensure that they all have the expected config version.
	for i := 0; i < int(redundancyLevel); i++ {
		// Note down the node ids to be ensured
		candidateNodeIds := make([]int, 0, redundancyLevel)
		for _, nodesInNodegroup := range nodesGroupedByNodegroups {
			candidateNodeIds = append(candidateNodeIds, nodesInNodegroup[i])
		}

		var nodesWithOldConfig []int
		// Check if the data nodes have the expected config version
		for _, nodeID := range candidateNodeIds {
			nodeConfigGeneration, err := mgmClient.GetConfigVersion(nodeID)
			if err != nil {
				return errorWhileProcessing(err)
			}

			if wantedGeneration != nodeConfigGeneration {
				// data node runs with an old versioned config
				nodesWithOldConfig = append(nodesWithOldConfig, nodeID)
			}
		}

		if len(nodesWithOldConfig) > 0 {
			// Stop all the data nodes that has old config version
			klog.Infof("Identified %d data node(s) with old config version : %v",
				len(nodesWithOldConfig), nodesWithOldConfig)

			err := mgmClient.StopNodes(nodesWithOldConfig)
			if err != nil {
				klog.Infof("Error stopping data nodes %v : %v", nodesWithOldConfig, err)
				return errorWhileProcessing(err)
			}

			// The data nodes have started to stop.
			// Exit here and allow them to be restarted by the statefulset controllers.
			// Continue syncing once they are up, in a later reconciliation loop.
			klog.Infof("The data nodes %v, identified with old config version, are being restarted", nodesWithOldConfig)
			// Stop processing. Reconciliation will continue
			// once the StatefulSet is fully ready again.
			return finishProcessing()
		}

		klog.Infof("The data nodes %v have desired config version %d", candidateNodeIds, wantedGeneration)
	}

	// All data nodes have the desired config version. Continue with rest of the sync process.
	klog.Info("All data nodes have the desired config version")
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
		pod, err := sc.podLister.Pods(sc.ndb.Namespace).Get(podName)
		if err != nil {
			klog.Errorf("Failed to find Management server with node id '%d' and pod '%s/%s' : %s",
				nodeId, sc.ndb.Namespace, podName, err)
			return nil, err
		}
		connectstring = fmt.Sprintf("%s:%d", pod.Status.PodIP, 1186)
		klog.V(2).Infof("Using pod %s/%s for management server with node id %d", sc.ndb.Namespace, podName, nodeId)
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
	klog.Infof("Ensuring Management Server(s) have the desired config version, %d", wantedGeneration)

	// Management servers have the first one/two node ids
	for nodeID := 1; nodeID <= (int)(sc.ndb.GetManagementNodeCount()); nodeID++ {
		mgmClient, err := sc.connectToManagementServer(nodeID)
		if err != nil {
			return errorWhileProcessing(err)
		}

		version, err := mgmClient.GetConfigVersion()
		if err != nil {
			klog.Error("GetConfigVersion failed :", err)
			return errorWhileProcessing(err)
		}

		klog.Infof("Config version of the Management server(nodeId=%d) : %d", nodeID, version)

		if version == wantedGeneration {
			klog.Infof("Management server(nodeId=%d) has the desired config version", nodeID)

			// This Management Server already has the desired config version.
			mgmClient.Disconnect()
			continue
		}

		klog.Infof("Management server(nodeId=%d) does not have desired config version", nodeID)

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
		klog.Infof("Management server(nodeId=%d) is being restarted with the desired configuration", nodeID)
		// Stop processing. Reconciliation will continue
		// once the StatefulSet is fully ready again.
		return finishProcessing()
	}

	// All Management Servers have the latest config.
	// Continue further processing and sync.
	klog.Info("All management nodes have the desired config version")
	return continueProcessing()
}

// checkPodsReadiness checks if all the pods owned the NdbCluster
// resource are ready. The sync will continue only if all the pods are ready.
func (sc *SyncContext) checkPodsReadiness() syncResult {

	// List all pods owned by NdbCluster resource
	pods, err := sc.podLister.Pods(sc.ndb.Namespace).List(labels.SelectorFromSet(sc.ndb.GetLabels()))
	if err != nil {
		klog.Errorf("Failed to list pods with selector %q. Error : %v",
			labels.Set(sc.ndb.GetLabels()).String(), err)
		return errorWhileProcessing(err)
	}

	// Check if all the pods are ready
	for _, pod := range pods {
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady {
				klog.V(2).Infof("Pod : %q Ready : %q",
					getNamespacedName(pod.GetObjectMeta()), condition.Status)
				if condition.Status != corev1.ConditionTrue {
					klog.Infof("Some pods owned by the NdbCluster resource %q are not ready yet",
						getNamespacedName(sc.ndb))
					// Stop syncing. NdbCluster resource will be later re-queued
					// once the deployment or the statefulset that controls this
					// pod becomes ready.
					return finishProcessing()
				}
			}
		}
	}

	klog.Infof("All pods owned by the NdbCluster resource %q are ready", getNamespacedName(sc.ndb))
	// Allow sync to continue further
	return continueProcessing()
}

// retrieveClusterStatus gets the cluster status from the
// Management Server and stores it in the SyncContext
func (sc *SyncContext) retrieveClusterStatus() (mgmapi.ClusterStatus, error) {

	mgmClient, err := sc.connectToManagementServer()
	if err != nil {
		return nil, err
	}
	defer mgmClient.Disconnect()

	// check if management nodes report a degraded cluster state
	cs, err := mgmClient.GetStatus()
	if err != nil {
		klog.Errorf("Error getting cluster status from management server: %s", err)
		return nil, err
	}

	sc.clusterState = cs

	return cs, nil
}

// ensureAllResources creates all K8s resources required for running the
// MySQL Cluster if they do no exist already. Resource creation needs to
// be idempotent just like any other step in the syncHandler. The config
// in the config map is used by this step rather than the spec of the
// NdbCluster resource to ensure a consistent setup across multiple
// reconciliation loops.
func (sc *SyncContext) ensureAllResources(ctx context.Context) syncResult {

	// bool to track if any resource was created as a part of this step
	allResourcesExist := true

	// helper method to report and handle the status of the resource
	handleResourceStatus := func(resourceExists bool, resourceName string) {
		if !resourceExists {
			klog.Infof("Created resource : %s", resourceName)
			allResourcesExist = false
		}
	}

	var err error
	var resourceExists bool

	// create all services required by the StatefulSets running the
	// MySQL Cluster nodes and to expose the services offered by those nodes.
	if _, resourceExists, err = sc.ensureServices(ctx); err != nil {
		return errorWhileProcessing(err)
	}
	handleResourceStatus(resourceExists, "Services")

	// create pod disruption budgets
	if _, resourceExists, err = sc.ensurePodDisruptionBudget(); err != nil {
		return errorWhileProcessing(err)
	}
	handleResourceStatus(resourceExists, "Pod Disruption Budgets")

	// enusure config map
	var cm *corev1.ConfigMap
	if cm, resourceExists, err = sc.configMapController.EnsureConfigMap(sc); err != nil {
		return errorWhileProcessing(err)
	}
	handleResourceStatus(resourceExists, "Config Map")

	// Create a new ResourceContext
	configAsString, err := resources.GetConfigFromConfigMapObject(cm)
	if err != nil {
		// less likely to happen as the config value cannot go missing at this point
		return errorWhileProcessing(err)
	}

	if sc.resourceContext, err = resources.NewResourceContextFromConfiguration(configAsString); err != nil {
		// less likely to happen as the only possible error is a config
		// parse error, and the configString was generated by the operator
		return errorWhileProcessing(err)
	}

	// create the management stateful set if it doesn't exist
	if _, resourceExists, err = sc.ensureManagementServerStatefulSet(ctx); err != nil {
		return errorWhileProcessing(err)
	}
	handleResourceStatus(resourceExists, "StatefulSet for Management Nodes")

	// create the data node stateful set if it doesn't exist
	if sc.dataNodeSfSet, resourceExists, err = sc.ensureDataNodeStatefulSet(ctx); err != nil {
		return errorWhileProcessing(err)
	}
	handleResourceStatus(resourceExists, "StatefulSet for Data Nodes")

	// MySQL Server deployment will be created only if required.
	// For now, just verify that if it exists, it is indeed owned by the NdbCluster resource.
	if sc.mysqldDeployment, err = sc.validateMySQLServerDeployment(); err != nil {
		return errorWhileProcessing(err)
	}

	if allResourcesExist {
		// All resources already existed before this sync loop
		klog.Infof("All resources exist already")
		return continueProcessing()
	}

	// All or some resources did not exist before this sync loop
	// and were created just now.
	klog.Infof("Some resources were just created. So, wait for them to become ready.")
	// Do not take any further action as the resources like pods
	// will need some time to get ready. Reconciliation will
	// continue once all the pods are ready.
	return finishProcessing()
}

// sync updates the MySQL Cluster configuration running inside
// the K8s Cluster based on the NdbCluster spec. This is the
// core reconciliation loop, and the complete sync takes place
// over multiple calls.
func (sc *SyncContext) sync(ctx context.Context) syncResult {

	// Multiple resources are required to start
	// and run the MySQL Cluster in K8s. Create
	// them if they do not exist yet.
	// TODO: Update ndb.Status with a creationTimestamp?
	//       All subsequent sync loop could just check that and skip ensuring resources
	//       Handle individual resources getting deleted.
	if sr := sc.ensureAllResources(ctx); sr.stopSync() {
		if err := sr.getError(); err != nil {
			klog.Errorf("Failed to ensure that all the required resources exist. Error : %v", err)
		}
		return sr
	}

	// All resources already exist or were created in a previous reconciliation loop.
	// Continue further only if all the pods are ready.
	if sr := sc.checkPodsReadiness(); sr.stopSync() {
		// Pods are not ready => The MySQL Cluster is not fully up yet.
		// Any further config changes cannot be processed until the pods are ready.
		return sr
	}

	// Resources exist and the pods(MySQL Cluster nodes) are ready.
	// Continue further only if the MySQL Cluster is healthy.
	// (i.e.) Only if all MySQL Cluster nodes are connected and running.
	// TODO: Check if this is redundant once the Readiness probes for ndbd/mgmd are implemented
	clusterState, err := sc.retrieveClusterStatus()
	if err != nil {
		// An error occurred when attempting to retrieve MySQL Cluster
		// status. The error would have been printed to log already,
		// so, just return the error.
		return errorWhileProcessing(err)
	}

	if !clusterState.IsHealthy() {
		// All/Some MySQL Cluster nodes are not ready yet. Requeue sync.
		klog.Infof("Some MySQL Cluster nodes are not ready yet")
		return requeueInSeconds(5)
	}

	// Resources, pods are ready and the MySQL Cluster is healthy.
	// Before starting to handle any new changes from the Ndb
	// Custom object, verify that the MySQL Cluster is in sync
	// with the current config in the config map. This is to avoid
	// applying config changes midway through a previous config
	// change. This also means that this entire reconciliation
	// will be spent only on this verification. If the MySQL
	// Cluster has the expected config, the K8s config map will be
	// updated with the new config, specified by the Ndb object,
	// at the end of this loop. The new changes will be applied to
	// the MySQL Cluster starting from the next reconciliation loop.

	// First pass of MySQL Server reconciliation.
	// If any scale down was requested, it will be handled in this pass.
	// This is done separately to ensure that the MySQL Servers are shut
	// down before possibly reducing the number of API sections in config.
	if sr := sc.mysqldController.HandleScaleDown(ctx, sc); sr.stopSync() {
		return sr
	}

	// make sure management server(s) have the correct config version
	if sr := sc.ensureManagementServerConfigVersion(); sr.stopSync() {
		return sr
	}

	// make sure all data nodes have the correct config version
	// data nodes a restarted with respect to
	if sr := sc.ensureDataNodeConfigVersion(); sr.stopSync() {
		return sr
	}

	// Second pass of MySQL Server reconciliation
	// Reconcile the rest of spec/config change in MySQL Server Deployment
	if sr := sc.mysqldController.ReconcileDeployment(ctx, sc); sr.stopSync() {
		return sr
	}

	// At this point, the MySQL Cluster is in sync with the configuration in the config map.
	// The configuration in the config map has to be checked to see if it is still the
	// desired config specified in the Ndb object.
	klog.Infof("The generation of the config in config map : \"%d\"", sc.resourceContext.ConfigGeneration)

	// calculate the hash of the new config
	newConfigHash, err := sc.ndb.CalculateNewConfigHash()
	if err != nil {
		klog.Errorf("Error calculating hash of incoming Ndb resource.")
		return errorWhileProcessing(err)
	}

	// Check if configuration in config map is still the desired from the Ndb CRD
	hasPendingConfigChanges := sc.resourceContext.ConfigHash != newConfigHash
	if hasPendingConfigChanges {
		// The Ndb object spec has changed - patch the config map
		klog.Info("Config in NdbCluster spec is different from existing config in config map")
		_, err := sc.configMapController.PatchConfigMap(sc.ndb, sc.resourceContext)
		if err != nil {
			klog.Infof("Failed to patch config map: %s", err)
			return errorWhileProcessing(err)
		}
	}

	// Update the status of the Ndb resource to reflect the state of any changes applied
	err = sc.updateNdbClusterStatus(hasPendingConfigChanges)
	if err != nil {
		klog.Errorf("Updating status failed: %v", err)
		return errorWhileProcessing(err)
	}

	if hasPendingConfigChanges {
		// Only the config map was updated during this loop.
		// The config changes still need to be applied to the MySQL Cluster.
		return requeueInSeconds(0)
	}

	return finishProcessing()
}

// updateNdbClusterStatus updates the status of the NdbCluster object and
// sends out an event if the object is already in syn with the MySQL Cluster
func (sc *SyncContext) updateNdbClusterStatus(hasPendingConfigChanges bool) error {

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
			klog.Infof("Updating the NdbCluster resource %q processed generation from %d to %d",
				getNamespacedName(sc.ndb), ndb.Status.ProcessedGeneration, ndb.ObjectMeta.Generation-1)
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
			klog.Infof("Updating the NdbCluster resource %q processed generation from %d to %d",
				getNamespacedName(sc.ndb), ndb.Status.ProcessedGeneration, ndb.ObjectMeta.Generation)
			ndb.Status.ProcessedGeneration = ndb.ObjectMeta.Generation
		}
	}

	// Set the time of this status update
	ndb.Status.LastUpdate = metav1.NewTime(time.Now())
	ndbClusterInterface := sc.ndbClientset().MysqlV1alpha1().NdbClusters(ndb.Namespace)

	updateErr := wait.ExponentialBackoff(retry.DefaultBackoff, func() (done bool, err error) {

		ndb, err = ndbClusterInterface.UpdateStatus(context.TODO(), ndb, metav1.UpdateOptions{})
		if err == nil {
			return true, nil
		}
		if !apierrors.IsConflict(err) {
			return false, err
		}

		updated, err := ndbClusterInterface.Get(context.TODO(), ndb.Name, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("Failed to get NdbCluster resource %q: %v", getNamespacedName(sc.ndb), err)
			return false, err
		}
		ndb = updated.DeepCopy()
		return false, nil
	})

	if updateErr != nil {
		klog.Errorf("Failed to update NdbCluster resource %q : %v", getNamespacedName(sc.ndb), updateErr)
		return updateErr
	}

	// Record an SyncSuccess event as the MySQL Cluster specification has been
	// successfully synced with the spec of Ndb object and the status has been updated.
	sc.recorder.Eventf(ndb, nil,
		corev1.EventTypeNormal, ReasonSyncSuccess, ActionSynced, MessageSyncSuccess)

	return nil
}
