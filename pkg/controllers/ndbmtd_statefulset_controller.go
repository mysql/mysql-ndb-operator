// Copyright (c) 2022, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"errors"
	"fmt"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller"
	"github.com/mysql/ndb-operator/pkg/mgmapi"
	"github.com/mysql/ndb-operator/pkg/mysqlclient"
	"github.com/mysql/ndb-operator/pkg/resources"
	"github.com/mysql/ndb-operator/pkg/resources/statefulset"

	"k8s.io/client-go/kubernetes"
	listerappsv1 "k8s.io/client-go/listers/apps/v1"
	listerscorev1 "k8s.io/client-go/listers/core/v1"
	klog "k8s.io/klog/v2"
)

type ndbmtdStatefulSetController struct {
	ndbNodeStatefulSetImpl
}

// newNdbmtdStatefulSetController creates a new ndbmtdStatefulSetController
func newNdbmtdStatefulSetController(client kubernetes.Interface,
	statefulSetLister listerappsv1.StatefulSetLister, secretLister listerscorev1.SecretLister) *ndbmtdStatefulSetController {
	return &ndbmtdStatefulSetController{
		ndbNodeStatefulSetImpl{
			client:             client,
			statefulSetLister:  statefulSetLister,
			ndbNodeStatefulset: statefulset.NewNdbmtdStatefulSet(secretLister),
		},
	}
}

const (
	AddNodeOnlineInProgress = ndbcontroller.GroupName + "/add-node-online-in-progress"
)

// startNewDataNodes scales up the ndbmtd statefulset to start the new data nodes
func (nssc *ndbmtdStatefulSetController) startNewDataNodes(ctx context.Context, sc *SyncContext) syncResult {
	ndbmtdCount := sc.configSummary.NumOfDataNodes
	ndbmtdSfset := sc.dataNodeSfSet
	if *(ndbmtdSfset.Spec.Replicas) == ndbmtdCount {
		// Statefulset already UpToDate
		klog.Infof("New data nodes have been started")
		return continueProcessing()
	}

	// Add Data Node Online in progress.
	// At this point, the config is patched and a rolling restart
	// of all the existing nodes on the system is complete. Start
	// the new data nodes by scaling up the data node statefulset.
	updatedSfset := ndbmtdSfset.DeepCopy()
	updatedSfset.Spec.Replicas = &ndbmtdCount
	return nssc.patchStatefulSet(ctx, ndbmtdSfset, updatedSfset)
}

// createNodeGroups inducts the new data nodes into the MySQL Cluster by creating nodegroups on them.
func (nssc *ndbmtdStatefulSetController) createNodeGroups(sc *SyncContext) syncResult {
	// Connect to the Management Server
	mgmClient, err := mgmapi.NewMgmClient(sc.ndb.GetConnectstring())
	if err != nil {
		klog.Errorf("Failed to connect to Management Server : %s", err)
		return errorWhileProcessing(err)
	}
	defer mgmClient.Disconnect()

	clusterStatus, err := mgmClient.GetStatus()
	if err != nil {
		klog.Errorf("Error getting cluster status from management server: %s", err)
		return errorWhileProcessing(err)
	}

	// Get the sorted list of all new nodes whose nodegroup is 65536
	newDataNodeIds := clusterStatus.GetConnectedDataNodesWithNodeGroup(mgmapi.NodeGroupNewConnectedDataNode)
	if newDataNodeIds == nil {
		// All nodes are already inducted into the MySQL Cluster
		klog.Info("Nodegroups exist for newly added data nodes")
		return continueProcessing()
	}

	// Induct the new nodes by creating nodegroups one by one
	numberOfNodesPerNodeGroup := sc.configSummary.RedundancyLevel
	var nodeIds []int
	for _, nodeId := range newDataNodeIds {
		nodeIds = append(nodeIds, nodeId)
		if len(nodeIds) == int(numberOfNodesPerNodeGroup) {
			// Create a new nodegroup
			ng, err := mgmClient.CreateNodeGroup(nodeIds)
			if err != nil {
				klog.Errorf("Failed to create nodegroup for nodes %v : %s", nodeIds, err)
				return errorWhileProcessing(err)
			}

			klog.Infof("Created nodegroup '%d' with data nodes %v", ng, nodeIds)

			// reset nodeIds for the next iteration
			nodeIds = nil
		}
	}

	// Done creating nodegroups
	klog.Info("Successfully created nodegroups for newly added data nodes")
	return continueProcessing()
}

// reorgNdbTables runs the required SQL queries to redistribute NDB
// table data across all data nodes including the newly added nodes.
func (nssc *ndbmtdStatefulSetController) reorgNdbTables(ctx context.Context, sc *SyncContext) syncResult {
	// Extract ndb operator mysql user password.
	nc := sc.ndb
	operatorSecretName := resources.GetMySQLNDBOperatorPasswordSecretName(nc)
	operatorPassword, err := NewMySQLUserPasswordSecretInterface(nssc.client).ExtractPassword(ctx, nc.Namespace, operatorSecretName)
	if err != nil {
		klog.Errorf("Failed to extract ndb operator password from the secret")
		return errorWhileProcessing(err)
	}

	// Connect to the 0th MySQL Pod to perform reorg partition and optimize
	mysqlClient, err := mysqlclient.ConnectToStatefulSet(sc.mysqldSfset, "", operatorPassword)
	if err != nil {
		return errorWhileProcessing(err)
	}

	// Get the list of all NDB tables
	query := "SELECT TABLE_SCHEMA, TABLE_NAME FROM " + mysqlclient.DbInformationSchema + ".TABLES WHERE ENGINE = 'NDBCLUSTER'"
	rows, err := mysqlClient.QueryContext(ctx, query)
	if err != nil {
		klog.Errorf("Failed to execute query %q : %s", query, err)
		return errorWhileProcessing(err)
	}

	// Run reorg and optimize for all tables, one by one.
	klog.Infof("Redistributing NDB data among all data nodes, including the new ones")
	var tableSchema, tableName string
	for rows.Next() {
		if err = rows.Scan(&tableSchema, &tableName); err != nil {
			klog.Errorf("Failed to scan list of NDB tables : %s", err)
			return errorWhileProcessing(err)
		}

		// Check if the table is already reorganized
		query = "SELECT COUNT(DISTINCT current_primary) " +
			"FROM ndbinfo.dictionary_tables AS t, ndbinfo.table_fragments AS f " +
			"WHERE t.table_id = f.table_id AND t.database_name=? AND t.table_name=?"
		row := mysqlClient.QueryRowContext(ctx, query, tableSchema, tableName)
		var distributedNodeCount int32
		if err = row.Scan(&distributedNodeCount); err != nil {
			klog.Errorf("Query '%s' failed : %s", query, err)
			return errorWhileProcessing(err)
		}
		tableFullName := fmt.Sprintf("%s.%s", tableSchema, tableName)
		if distributedNodeCount == sc.configSummary.NumOfDataNodes {
			// Table already reorganized
			klog.Infof("Table %q has already been redistributed", tableFullName)
			continue
		}

		// Run ALTER TABLE ... ALGORITHM=INPLACE, REORGANIZE PARTITION
		query = fmt.Sprintf("ALTER TABLE %s ALGORITHM=INPLACE, REORGANIZE PARTITION", tableFullName)
		klog.Infof("Running '%s'", query)
		if _, err = mysqlClient.ExecContext(ctx, query); err != nil {
			klog.Errorf("Query '%s' failed : %s", query, err)
			return errorWhileProcessing(err)
		}

		// Run OPTIMIZE TABLE
		query = fmt.Sprintf("OPTIMIZE TABLE %s", tableFullName)
		klog.Infof("Running '%s'", query)
		row = mysqlClient.QueryRowContext(ctx, query)
		var dummy, result, msg string
		if err = row.Scan(&dummy, &dummy, &result, &msg); err == nil && result == "error" {
			err = errors.New(msg)
		}

		if err != nil {
			klog.Errorf("Query '%s' failed : %s", query, err)
			return errorWhileProcessing(err)
		}
	}
	klog.Infof("Successfully redistributed all NDB data")

	// Done running reorg and optimize table
	return continueProcessing()
}

// handleAddNodeOnline scales up the data node statefulset and
// then creates nodegroups for the newly started data nodes.
func (nssc *ndbmtdStatefulSetController) handleAddNodeOnline(ctx context.Context, sc *SyncContext) syncResult {
	ndbmtdSfset := sc.dataNodeSfSet
	if _, exists := ndbmtdSfset.GetAnnotations()[AddNodeOnlineInProgress]; !exists {
		// Add node online not in progress
		return continueProcessing()
	}

	// Add node online in progress
	// Scale up the data nodes
	if sr := nssc.startNewDataNodes(ctx, sc); sr.stopSync() {
		return sr
	}

	// Create node groups
	if sr := nssc.createNodeGroups(sc); sr.stopSync() {
		return sr
	}

	// Reorg all NDB tables
	if sr := nssc.reorgNdbTables(ctx, sc); sr.stopSync() {
		return sr
	}

	// Delete AddNodeOnlineInProgress annotation as add node online procedure is now complete
	updatedSfset := ndbmtdSfset.DeepCopy()
	delete(updatedSfset.GetAnnotations(), AddNodeOnlineInProgress)
	return nssc.patchStatefulSet(ctx, ndbmtdSfset, updatedSfset)
}
