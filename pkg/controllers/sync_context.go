// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	ndbclientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
	ndblisters "github.com/mysql/ndb-operator/pkg/generated/listers/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/mgmapi"
	"github.com/mysql/ndb-operator/pkg/ndbconfig"
)

// SyncContext stores all information collected in/for a single run of syncHandler
type SyncContext struct {
	// Summary of the MySQL Cluster configuration extracted from the current configmap
	configSummary *ndbconfig.ConfigSummary

	// Workload resources
	mgmdNodeSfset *appsv1.StatefulSet
	dataNodeSfSet *appsv1.StatefulSet
	mysqldSfset   *appsv1.StatefulSet

	ndb *v1.NdbCluster

	// controller handling creation and changes of resources
	mysqldController    *MySQLDStatefulSetController
	mgmdController      NdbStatefulSetControlInterface
	ndbmtdController    *ndbmtdStatefulSetController
	configMapController ConfigMapControlInterface
	serviceController   ServiceControlInterface
	pdbController       PodDisruptionBudgetControlInterface

	kubernetesClient kubernetes.Interface
	ndbClient        ndbclientset.Interface
	ndbsLister       ndblisters.NdbClusterLister
	podLister        listerscorev1.PodLister
	serviceLister    listerscorev1.ServiceLister

	// bool flag to control the NdbCluster status processedGeneration value
	syncSuccess bool

	// recorder is an event recorder for recording Event resources to the Kubernetes API.
	recorder events.EventRecorder
}

func (sc *SyncContext) kubeClientset() kubernetes.Interface {
	return sc.kubernetesClient
}

func (sc *SyncContext) ndbClientset() ndbclientset.Interface {
	return sc.ndbClient
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

// ensureManagementServerStatefulSet creates the StatefulSet for
// Management Server in the K8s Server if they don't exist yet.
func (sc *SyncContext) ensureManagementServerStatefulSet(
	ctx context.Context) (mgmdStatefulSet *appsv1.StatefulSet, existed bool, err error) {
	return sc.mgmdController.EnsureStatefulSet(ctx, sc)
}

// ensureDataNodeStatefulSet creates the StatefulSet for
// Data Nodes in the K8s Server if they don't exist yet.
func (sc *SyncContext) ensureDataNodeStatefulSet(
	ctx context.Context) (ndbmtdStatefulSet *appsv1.StatefulSet, existed bool, err error) {
	return sc.ndbmtdController.EnsureStatefulSet(ctx, sc)
}

// validateMySQLServerStatefulSet retrieves the MySQL Server statefulset from K8s.
// If the statefulset exists, it verifies if it is owned by the NdbCluster resource.
func (sc *SyncContext) validateMySQLServerStatefulSet() (*appsv1.StatefulSet, error) {
	return sc.mysqldController.GetStatefulSet(sc)
}

// ensurePodDisruptionBudgets creates PodDisruptionBudgets for data nodes
func (sc *SyncContext) ensurePodDisruptionBudget(ctx context.Context) (existed bool, err error) {
	// ensure ndbmtd PDB
	return sc.pdbController.EnsurePodDisruptionBudget(
		ctx, sc, sc.ndbmtdController.GetTypeName())
}

// reconcileManagementNodeStatefulSet patches the Management Node
// StatefulSet with the spec from the latest generation of NdbCluster.
func (sc *SyncContext) reconcileManagementNodeStatefulSet(ctx context.Context) syncResult {
	return sc.mgmdController.ReconcileStatefulSet(ctx, sc.mgmdNodeSfset, sc)
}

// reconcileDataNodeStatefulSet patches the Data Node StatefulSet
// with the spec from the latest generation of NdbCluster.
func (sc *SyncContext) reconcileDataNodeStatefulSet(ctx context.Context) syncResult {
	return sc.ndbmtdController.ReconcileStatefulSet(ctx, sc.dataNodeSfSet, sc)
}

// ensurePodVersion checks if the pod with the given name has the desired pod version.
// If the pod is running with an old spec version, it will be deleted allowing the
// statefulSet controller to restart it with the latest pod definition.
func (sc *SyncContext) ensurePodVersion(
	ctx context.Context, namespace, podName, desiredPodVersion, podDescription string) (
	podDeleted bool, err error) {

	// Retrieve the pod and extract its version
	pod, err := sc.podLister.Pods(namespace).Get(podName)
	if err != nil {
		klog.Errorf("Failed to find pod '%s/%s' running %s : %s", namespace, podName, podDescription, err)
		return false, err
	}

	podVersion := pod.GetLabels()["controller-revision-hash"]
	klog.Infof("Version of %s's pod : %s", podDescription, podVersion)

	if podVersion == desiredPodVersion {
		klog.Infof("%s has the desired version of podSpec", podDescription)
		return false, nil
	}

	klog.Infof("%s does not have desired version of podSpec", podDescription)

	// The Pod does not have the desired version.
	// Delete it and let the statefulset controller restart it with the latest pod definition.
	err = sc.kubeClientset().CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("Failed to delete pod '%s/%s' running %s : %s", namespace, podName, podDescription, err)
		return false, err
	}

	// The pod has been deleted.
	klog.Infof("Pod running %s is being restarted with the desired configuration", podDescription)
	return true, nil
}

// ensureDataNodePodVersion checks if all the Data Node pods
// have the latest podSpec defined by the StatefulSet. If not, it safely
// restarts them without affecting the availability of MySQL Cluster.
//
// The method chooses one data node per nodegroup and checks their PodSpec
// version. The nodes that have an outdated PodSpec version among them
// will be deleted together allowing the K8s StatefulSet controller to
// restart them with the latest pod definition along with the latest
// config available in the config map. When the chosen data nodes are
// being restarted and updated, any further reconciliation is stopped, and
// is resumed only after the restarted data nodes become ready. At any
// given time, only one data node per group will be affected by this
// maneuver, ensuring MySQL Cluster's availability.
func (sc *SyncContext) ensureDataNodePodVersion(ctx context.Context) syncResult {
	ndbmtdSfset := sc.dataNodeSfSet
	if statefulsetUpdateComplete(ndbmtdSfset) {
		// All data nodes have the desired pod version.
		// Continue with rest of the sync process.
		klog.Info("All Data node pods are up-to-date and ready")
		return continueProcessing()
	}

	desiredPodRevisionHash := ndbmtdSfset.Status.UpdateRevision
	klog.Infof("Ensuring Data Node pods have the desired podSpec version, %s", desiredPodRevisionHash)

	// Get the node and nodegroup details via clusterStatus
	mgmClient, err := mgmapi.NewMgmClient(sc.ndb.GetConnectstring())
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
	// ensure that they all have the latest Pod definition.
	redundancyLevel := sc.configSummary.RedundancyLevel
	for i := 0; i < int(redundancyLevel); i++ {
		// Note down the node ids to be ensured
		candidateNodeIds := make([]int, 0, redundancyLevel)
		for _, nodesInNodegroup := range nodesGroupedByNodegroups {
			candidateNodeIds = append(candidateNodeIds, nodesInNodegroup[i])
		}

		// Check the pods running MySQL Cluster nodes with candidateNodeIds
		// and delete them if they have an older pod definition.
		var nodesBeingUpdated []int
		for _, nodeId := range candidateNodeIds {
			// Generate the pod name using nodeId.
			// Data node with nodeId 'i' runs in a pod with ordinal index 'i-1-numberOfMgmdNodes'
			ndbmtdPodName := fmt.Sprintf(
				"%s-%d", ndbmtdSfset.Name, nodeId-1-int(sc.configSummary.NumOfManagementNodes))

			// Check the pod version and delete it if its outdated
			podDeleted, err := sc.ensurePodVersion(
				ctx, ndbmtdSfset.Namespace, ndbmtdPodName, desiredPodRevisionHash,
				fmt.Sprintf("Data Node(nodeId=%d)", nodeId))
			if err != nil {
				return errorWhileProcessing(err)
			}

			if podDeleted {
				nodesBeingUpdated = append(nodesBeingUpdated, nodeId)
			}
		}

		if len(nodesBeingUpdated) > 0 {
			// The outdated data nodes are being updated.
			// Exit here and allow them to be restarted by the statefulset controllers.
			// Continue syncing once they are up, in a later reconciliation loop.
			klog.Infof("The data nodes %v, identified with old pod version, are being restarted", nodesBeingUpdated)
			// Stop processing. Reconciliation will continue
			// once the StatefulSet is fully ready again.
			return finishProcessing()
		}

		klog.Infof("The data nodes %v have the desired pod version %s", candidateNodeIds, desiredPodRevisionHash)
	}

	// Control will never reach here but to make compiler happy return continue.
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
	if sc.configSummary, err = ndbconfig.NewConfigSummary(cm.Data); err != nil {
		// less likely to happen as the only possible error is a config
		// parse error, and the configString was generated by the operator
		return errorWhileProcessing(err)
	}

	// First ensure that a operator password secret exists before creating statefulSet
	secretClient := NewMySQLUserPasswordSecretInterface(sc.kubernetesClient)
	if _, err := secretClient.EnsureNDBOperatorPassword(ctx, sc.ndb); err != nil {
		klog.Errorf("Failed to ensure ndb-operator password secret : %s", err)
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
	if initialSystemRestart && !statefulsetUpdateComplete(sc.mgmdNodeSfset) {
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
	if initialSystemRestart && !statefulsetUpdateComplete(sc.dataNodeSfSet) {
		// Data nodes are starting for the first time, and some of
		// them are not ready yet which implies that this
		// reconciliation was triggered only to update the NdbCluster
		// status. No need to proceed further.
		return finishProcessing()
	}

	// MySQL Server StatefulSet will be created only if required.
	// For now, just verify that if it exists, it is indeed owned by the NdbCluster resource.
	if sc.mysqldSfset, err = sc.validateMySQLServerStatefulSet(); err != nil {
		return errorWhileProcessing(err)
	}
	if sc.mysqldSfset != nil && !statefulsetUpdateComplete(sc.mysqldSfset) {
		// MySQL Server StatefulSet exists, but it is not complete yet
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
	// The data node StatefulSet should be ready and both the
	// mgmd and MySQL StatefulSets should be complete.
	if statefulsetUpdateComplete(sc.mgmdNodeSfset) &&
		statefulsetReady(sc.dataNodeSfSet) &&
		(sc.mysqldSfset == nil ||
			statefulsetUpdateComplete(sc.mysqldSfset)) {
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

// sync updates the configuration of the MySQL Cluster running
// inside the K8s Cluster based on the NdbCluster spec. This is
// the core reconciliation loop, and a complete update takes
// place over multiple sync calls.
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

	// Reconcile Management Server by updating the statefulSet definition.
	// Management StatefulSet uses the default RollingUpdate strategy and
	// the update will be rolled out by the controller once the StatefulSet
	// is patched.
	if sr := sc.reconcileManagementNodeStatefulSet(ctx); sr.stopSync() {
		return sr
	}
	klog.Info("All Management node pods are up-to-date and ready")

	// Reconcile Data Nodes by updating their statefulSet definition
	if sr := sc.reconcileDataNodeStatefulSet(ctx); sr.stopSync() {
		return sr
	}

	// Restart Data Node pods, if required, to update their definitions
	if sr := sc.ensureDataNodePodVersion(ctx); sr.stopSync() {
		return sr
	}

	// Second pass of MySQL Server reconciliation
	// Reconcile the rest of spec/config change in MySQL Server StatefulSet
	if sr := sc.mysqldController.ReconcileStatefulSet(ctx, sc); sr.stopSync() {
		return sr
	}

	// Handle online add data node request
	if sr := sc.ndbmtdController.handleAddNodeOnline(ctx, sc); sr.stopSync() {
		return sr
	}

	// Reconcile the Root user
	if sr := sc.mysqldController.reconcileRootUser(ctx, sc); sr.stopSync() {
		return sr
	}

	// At this point, the MySQL Cluster is in sync with the configuration in the config map.
	// The configuration in the config map has to be checked to see if it is still the
	// desired config specified in the Ndb object.
	klog.Infof("The generation of the config in the configMap : \"%d\"", sc.configSummary.NdbClusterGeneration)

	// Check if the config map has processed the latest NdbCluster Generation
	if sc.configSummary.NdbClusterGeneration != sc.ndb.Generation {
		// The Ndb object spec has changed - patch the config map
		klog.Info("A new generation of NdbCluster spec exists and the config map needs to be updated")
		if _, err := sc.configMapController.PatchConfigMap(ctx, sc); err != nil {
			return errorWhileProcessing(err)
		}
		// Only the config map was updated during this loop.
		// The next loop will actually start the sync
		return finishProcessing()
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
	ndbClusterInterface := sc.ndbClientset().MysqlV1().NdbClusters(sc.ndb.Namespace)
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
