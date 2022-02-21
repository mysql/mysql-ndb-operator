// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	ndbclientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
	ndblisters "github.com/mysql/ndb-operator/pkg/generated/listers/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/helpers"
	"github.com/mysql/ndb-operator/pkg/mgmapi"
	"github.com/mysql/ndb-operator/pkg/ndbconfig"
	"github.com/mysql/ndb-operator/pkg/resources"
)

// SyncContext stores all information collected in/for a single run of syncHandler
type SyncContext struct {
	// Summary of the MySQL Cluster configuration extracted from the current configmap
	configSummary *ndbconfig.ConfigSummary

	// Workload resources
	mgmdNodeSfset    *appsv1.StatefulSet
	dataNodeSfSet    *appsv1.StatefulSet
	mysqldDeployment *appsv1.Deployment

	ManagementServerPort int32
	ManagementServerIP   string

	ndb *v1alpha1.NdbCluster

	// controller handling creation and changes of resources
	mysqldController    DeploymentControlInterface
	mgmdController      StatefulSetControlInterface
	ndbdController      StatefulSetControlInterface
	configMapController ConfigMapControlInterface
	pdbController       PodDisruptionBudgetControlInterface

	controllerContext *ControllerContext
	ndbsLister        ndblisters.NdbClusterLister
	podLister         listerscorev1.PodLister
	serviceLister     listerscorev1.ServiceLister

	// bool flag to control the NdbCluster status processedGeneration value
	syncSuccess bool

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

// ensurePodDisruptionBudgets creates PodDisruptionBudgets for data nodes
func (sc *SyncContext) ensurePodDisruptionBudget(ctx context.Context) (existed bool, err error) {
	// ensure ndbd PDB
	return sc.pdbController.EnsurePodDisruptionBudget(
		ctx, sc, sc.ndbdController.GetTypeName())
}

// ensureDataNodeConfigVersion checks if all the data nodes have the
// desired configuration. If not, it safely restarts them without
// affecting the availability of MySQL Cluster.
//
// The method chooses one data node per nodegroup and checks their config
// version. The nodes that have an outdated config version will be stopped
// together via the mgm client. The K8s statefulset controller will then
// restart those nodes using the latest config available in the config map.
// When the chosen data nodes are being restarted in this way, any further
// reconciliation is stopped, and is resumed only after the restarted data
// nodes become ready. At any given time, only one data node per group will
// be affected by this maneuver thus ensuring MySQL Cluster's availability.
func (sc *SyncContext) ensureDataNodeConfigVersion() syncResult {

	// Get the node and nodegroup details via clusterStatus
	mgmClient, err := sc.connectToManagementServer()
	if err != nil {
		return errorWhileProcessing(err)
	}
	defer mgmClient.Disconnect()
	clusterStatus, err := mgmClient.GetStatus()
	if err != nil {
		klog.Errorf("Error getting cluster status from management server: %s", err)
		return errorWhileProcessing(err)
	}

	// Group the nodes based on nodegroup.
	// The node ids are sorted within the sub arrays and the array
	// itself is sorted based on the node groups. Every sub array
	// will have RedundancyLevel number of node ids.
	nodesGroupedByNodegroups := clusterStatus.GetNodesGroupedByNodegroup()
	if nodesGroupedByNodegroups == nil {
		err := fmt.Errorf("internal error: could not extract nodes and node groups from cluster status")
		return errorWhileProcessing(err)
	}

	// Pick up the i'th node id from every sub array of
	// nodesGroupedByNodegroups during every iteration and
	// ensure that they all have the expected config version.
	wantedGeneration := sc.configSummary.ConfigGeneration
	redundancyLevel := sc.configSummary.RedundancyLevel
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

// ensureManagementServerConfigVersion checks if all the management nodes
// have the desired configuration. If not, it safely restarts them without
// affecting the availability of MySQL Cluster.
func (sc *SyncContext) ensureManagementServerConfigVersion() syncResult {
	wantedGeneration := sc.configSummary.ConfigGeneration
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

// ensureAllResources creates all K8s resources required for running the
// MySQL Cluster if they do no exist already. Resource creation needs to
// be idempotent just like any other step in the syncHandler. The config
// in the config map is used by this step rather than the spec of the
// NdbCluster resource to ensure a consistent setup across multiple
// reconciliation loops.
func (sc *SyncContext) ensureAllResources(ctx context.Context) syncResult {
	var err error
	var resourceExists bool

	// create all services required by the StatefulSets running the
	// MySQL Cluster nodes and to expose the services offered by those nodes.
	if _, resourceExists, err = sc.ensureServices(ctx); err != nil {
		return errorWhileProcessing(err)
	}
	if !resourceExists {
		klog.Info("Created resource : Services")
	}

	// create pod disruption budgets
	if resourceExists, err = sc.ensurePodDisruptionBudget(ctx); err != nil {
		return errorWhileProcessing(err)
	}
	if !resourceExists {
		klog.Info("Created resource : Pod Disruption Budgets")
	}

	// ensure config map
	var cm *corev1.ConfigMap
	if cm, resourceExists, err = sc.configMapController.EnsureConfigMap(ctx, sc); err != nil {
		return errorWhileProcessing(err)
	}
	if !resourceExists {
		klog.Info("Created resource : Config Map")
	}

	// Create a new ConfigSummary
	configAsString, err := resources.GetConfigFromConfigMapObject(cm)
	if err != nil {
		// less likely to happen as the config value cannot go missing at this point
		return errorWhileProcessing(err)
	}

	if sc.configSummary, err = ndbconfig.NewConfigSummary(configAsString); err != nil {
		// less likely to happen as the only possible error is a config
		// parse error, and the configString was generated by the operator
		return errorWhileProcessing(err)
	}

	// create the management stateful set if it doesn't exist
	if sc.mgmdNodeSfset, resourceExists, err = sc.ensureManagementServerStatefulSet(ctx); err != nil {
		return errorWhileProcessing(err)
	}
	if !resourceExists {
		// Management statefulset was just created.
		klog.Info("Created resource : StatefulSet for Management Nodes")
		// Wait for it to become ready before starting the data nodes.
		// Reconciliation will continue once all the pods in the statefulset are ready.
		klog.Infof("Reconciliation will continue after all the management nodes are ready.")
		return finishProcessing()
	}
	initialSystemRestart := sc.ndb.Status.ProcessedGeneration == 0
	if initialSystemRestart && !statefulsetReady(sc.mgmdNodeSfset) {
		// Management nodes are starting for the first time, and
		// one of them is not ready yet which implies that this
		// reconciliation was triggered only to update the
		// NdbCluster status. No need to start data nodes and
		// MySQL Servers yet.
		return finishProcessing()
	}

	// create the data node stateful set if it doesn't exist
	if sc.dataNodeSfSet, resourceExists, err = sc.ensureDataNodeStatefulSet(ctx); err != nil {
		return errorWhileProcessing(err)
	}
	if !resourceExists {
		// Data nodes statefulset was just created.
		klog.Info("Created resource : StatefulSet for Data Nodes")
		// Wait for it to become ready.
		// Reconciliation will continue once all the pods in the statefulset are ready.
		klog.Infof("Reconciliation will continue after all the data nodes are ready.")
		return finishProcessing()
	}
	if initialSystemRestart && !statefulsetReady(sc.dataNodeSfSet) {
		// Data nodes are starting for the first time, and some of
		// them are not ready yet which implies that this
		// reconciliation was triggered only to update the NdbCluster
		// status. No need to proceed further.
		return finishProcessing()
	}

	// MySQL Server deployment will be created only if required.
	// For now, just verify that if it exists, it is indeed owned by the NdbCluster resource.
	if sc.mysqldDeployment, err = sc.validateMySQLServerDeployment(); err != nil {
		return errorWhileProcessing(err)
	}
	if sc.mysqldDeployment != nil && !deploymentComplete(sc.mysqldDeployment) {
		// MySQL Server deployment exists, but it is not complete yet
		// which implies that this reconciliation was triggered only
		// to update the NdbCluster status. No need to proceed further.
		return finishProcessing()
	}

	// The StatefulSets already existed before this sync loop.
	// There is a rare chance that some other resources were created during
	// this sync loop as they were dropped by some other application other
	// than the operator. We can still continue processing in that case as
	// they will become immediately ready.
	klog.Infof("All resources exist")
	return continueProcessing()
}

// ensureWorkloadsReadiness checks if all the workloads created for the
// NdbCluster resource are ready. The sync is stopped if they are not ready.
func (sc *SyncContext) ensureWorkloadsReadiness() syncResult {
	// Both mgmd and data node StatefulSets should be ready and
	// the MySQL deployment should be complete if it exists.
	if statefulsetReady(sc.mgmdNodeSfset) &&
		statefulsetReady(sc.dataNodeSfSet) &&
		(sc.mysqldDeployment == nil ||
			deploymentComplete(sc.mysqldDeployment)) {
		klog.Infof("All workloads owned by the NdbCluster resource %q are ready", getNamespacedName(sc.ndb))
		return continueProcessing()
	}

	// Some workload is not ready yet => some pods are not ready yet
	klog.Infof("Some pods owned by the NdbCluster resource %q are not ready yet",
		getNamespacedName(sc.ndb))
	// Stop processing.
	// Reconciliation will continue when all the pods are ready.
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
	if sr := sc.ensureAllResources(ctx); sr.stopSync() {
		if err := sr.getError(); err != nil {
			klog.Errorf("Failed to ensure that all the required resources exist. Error : %v", err)
		}
		return sr
	}

	// All resources and workloads exist.
	// Continue further only if all the workloads are ready.
	if sr := sc.ensureWorkloadsReadiness(); sr.stopSync() {
		// Some workloads are not ready yet => The MySQL Cluster is not fully up yet.
		// Any further config changes cannot be processed until the pods are ready.
		return sr
	}

	// The workloads are ready => MySQL Cluster is healthy.
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
	klog.Infof("The generation of the config in config map : \"%d\"", sc.configSummary.ConfigGeneration)

	// calculate the hash of the new config
	newConfigHash, err := sc.ndb.CalculateNewConfigHash()
	if err != nil {
		klog.Errorf("Error calculating hash of incoming Ndb resource.")
		return errorWhileProcessing(err)
	}

	// Check if configuration in config map is still the desired from the Ndb CRD
	if sc.configSummary.ConfigHash != newConfigHash {
		// The Ndb object spec has changed - patch the config map
		klog.Info("Config in NdbCluster spec is different from existing config in config map")
		if _, err = sc.configMapController.PatchConfigMap(ctx, sc); err != nil {
			return errorWhileProcessing(err)
		}
		// Only the config map was updated during this loop.
		// The config changes still need to be applied to the MySQL Cluster.
		return requeueInSeconds(0)
	}

	// MySQL Cluster in sync with the NdbCluster spec
	sc.syncSuccess = true
	return finishProcessing()
}

// updateNdbClusterStatus updates the status of the SyncContext's NdbCluster resource in the K8s API Server
func (sc *SyncContext) updateNdbClusterStatus(ctx context.Context) (statusUpdated bool, err error) {

	// Use the DeepCopied NdbCluster resource to make the update
	nc := sc.ndb
	// Generate status with recent state of various resources
	status := sc.calculateNdbClusterStatus()

	// Update the status. Use RetryOnConflict to automatically handle
	// conflicts that can occur if the spec changes between the time
	// the controller gets the NdbCluster object and updates it.
	ndbClusterInterface := sc.ndbClientset().MysqlV1alpha1().NdbClusters(sc.ndb.Namespace)
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Check if the status has changed, if not the K8s update can be skipped
		if statusEqual(&nc.Status, status) {
			// Status up-to-date. No update required.
			return nil
		}

		// Update the status of NdbCluster
		status.DeepCopyInto(&nc.Status)
		// Send the update to K8s server
		klog.Infof("Updating the NdbCluster resource %q status", getNamespacedName(sc.ndb))
		_, updateErr := ndbClusterInterface.UpdateStatus(ctx, nc, metav1.UpdateOptions{})

		if updateErr == nil {
			// Status update succeeded
			statusUpdated = true
			return nil
		}

		// Get the latest version of the NdbCluster object from
		// the K8s API Server for retrying the update.
		var getErr error
		nc, getErr = ndbClusterInterface.Get(ctx, sc.ndb.Name, metav1.GetOptions{})
		if getErr != nil {
			klog.Errorf("Failed to get NdbCluster resource during status update %q: %v",
				getNamespacedName(sc.ndb), getErr)
			return getErr
		}

		// Return the updateErr. If it is a conflict error, RetryOnConflict
		// will retry the update with the latest version of the NdbCluster object.
		return updateErr
	})

	if err != nil {
		klog.Errorf("Failed to update the status of NdbCluster resource %q : %v",
			getNamespacedName(sc.ndb), err)
	}

	return statusUpdated, err
}
