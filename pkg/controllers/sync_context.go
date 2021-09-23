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

	// recorder is an event recorder for recording Event resources to the Kubernetes API.
	recorder events.EventRecorder

	// resource map stores the name of the resources already created and if they were created
	resourceMap map[string]bool
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
			sc.ndbClientset().MysqlV1alpha1().NdbClusters(ndb.Namespace).Update(context.TODO(), ndb, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}

		key := fmt.Sprintf("%s/%s", ndb.GetNamespace(), ndb.GetName())
		klog.V(4).Infof("Conflict updating Cluster labels. Getting updated Cluster %s from cache...", key)

		updated, err := sc.ndbClientset().MysqlV1alpha1().NdbClusters(ndb.Namespace).Get(context.TODO(), ndb.Name, metav1.GetOptions{})
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
			corev1.EventTypeWarning, ReasonResourceExists, ActionNone, msg)
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

// validateMySQLServerDeployment retrieves the MySQL Server deployment from K8s.
// If the deployment exists, it verifies if it is owned by the NdbCluster resource.
func (sc *SyncContext) validateMySQLServerDeployment(ctx context.Context) (*appsv1.Deployment, error) {

	nc := sc.ndb
	deployment, err := sc.mysqldController.GetDeployment(ctx, nc)
	if err != nil {
		// Failed to ensure the deployment
		return nil, err
	}

	if deployment == nil {
		// Deployment doesn't exist yet.
		// This is OK as it might be created later during sync.
		return nil, nil
	}

	// If the deployment is not controlled by Ndb resource,
	// log a warning to the event recorder and return the error message.
	if !metav1.IsControlledBy(deployment, sc.ndb) {
		err = fmt.Errorf(MessageResourceExists, deployment.Name)
		sc.recorder.Eventf(sc.ndb, nil,
			corev1.EventTypeWarning, ReasonResourceExists, ActionNone, err.Error())
		return nil, err
	}

	// deployment exists and is owned by NdbCluster
	return deployment, nil
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
			return requeueInSeconds(5)
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
		pod, err := sc.kubeClientset().CoreV1().Pods(sc.ndb.Namespace).Get(context.TODO(), podName, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("Management server with node id '%d' and pod '%s/%s' not found",
				nodeId, sc.ndb.Namespace, podName)
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
		return requeueInSeconds(5)
	}

	// All Management Servers have the latest config.
	// Continue further processing and sync.
	klog.Info("All management nodes have the desired config version")
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

// allResourcesExisted returns falls if any resource map is false (resource was created)
func (sc *SyncContext) allResourcesExisted() bool {

	retExisted := true
	for res, existed := range sc.resourceMap {
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
	if _, sc.resourceMap["labels"], err = sc.ensureClusterLabel(); err != nil {
		return false, err
	}

	// create services for management server and data node statefulsets
	// with respect to idempotency and atomicy service creation is always safe as it
	// only uses the immutable CRD name
	// service needs to be created and present when creating stateful sets
	if _, sc.resourceMap["services"], err = sc.ensureServices(); err != nil {
		return false, err
	}

	// create pod disruption budgets
	if _, sc.resourceMap["poddisruptionservice"], err = sc.ensurePodDisruptionBudget(); err != nil {
		return false, err
	}

	// create config map if not exist
	var cm *corev1.ConfigMap
	if cm, sc.resourceMap["configmap"], err = sc.configMapController.EnsureConfigMap(sc); err != nil {
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
	if _, sc.resourceMap["mgmstatefulset"], err = sc.ensureManagementServerStatefulSet(); err != nil {
		return false, err
	}

	// create the data node stateful set if it doesn't exist
	if sc.dataNodeSfSet, sc.resourceMap["datanodestatefulset"], err = sc.ensureDataNodeStatefulSet(); err != nil {
		return false, err
	}

	// MySQL Server deployment will be created only if required.
	// For now, just verify that if it exists, it is indeed owned by the NdbCluster resource.
	if sc.mysqldDeployment, err = sc.validateMySQLServerDeployment(context.TODO()); err != nil {
		return false, err
	}

	return sc.allResourcesExisted(), nil
}

// sync updates the MySQL Cluster configuration running inside
// the K8s Cluster based on the NdbCluster spec. This is the
// core reconciliation loop, and the complete sync takes place
// over multiple calls.
func (sc *SyncContext) sync(ctx context.Context) syncResult {

	// Multiple resources are required to start
	// and run the MySQL Cluster in K8s. Create
	// them if they do not exist yet.
	existed, err := sc.ensureAllResources()
	if err != nil {
		klog.Errorf("Failed to ensure that all the required resources exist. Error : %v", err)
		return errorWhileProcessing(err)
	}

	// TODO: Update ndb.Status with a creationTimestamp?
	//       All subsequent sync loop could just check that and skip ensuring resources
	//       Handle individual resources getting deleted.
	// All resources either exist or were created
	if !existed {
		// All or some resources did not exist before this sync loop
		// and were created just now. Do not take any further action
		// as the resources like pods will need some time to get ready.
		klog.Infof("Some resources were just created. So, wait for them to become ready.")
		return requeueInSeconds(5)
	}

	// All resources already exist or were created in a previous reconciliation loop.
	// Continue further only if all the pods are ready.
	if ready, _ := sc.checkPodStatus(); !ready {
		// Pods are not ready => The MySQL Cluster is not fully up yet.
		// Any further config changes cannot be processed until the pods are ready.
		klog.Infof("Some of the pods running MySQL Cluster nodes are not ready yet")
		return requeueInSeconds(5)
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

	// If this number of the members on the Cluster does not equal the
	// current desired replicas on the StatefulSet, we should update the
	// StatefulSet resource.
	// TODO : Check if this is necessary as this case is
	//        probably covered already by the previous step.
	if sc.resourceContext.NumOfDataNodes != uint32(*sc.dataNodeSfSet.Spec.Replicas) {
		klog.Infof("Updating NdbCluster resource %q : DataNodes=%d statefulSetReplicas=%d",
			getNamespacedName(sc.ndb), sc.ndb.Spec.NodeCount, *sc.dataNodeSfSet.Spec.Replicas)
		if sc.dataNodeSfSet, err = sc.ndbdController.Patch(sc.resourceContext, sc.ndb, sc.dataNodeSfSet); err != nil {
			// Requeue the item so we can attempt processing again later.
			// This could have been caused by a temporary network failure etc.
			return errorWhileProcessing(err)
		}
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
