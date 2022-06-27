// Copyright (c) 2020, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"

	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/mysqlclient"
	"github.com/mysql/ndb-operator/pkg/resources/statefulset"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

// handleRootHostChanges detects any changes made to spec.mysqld.rootHost in NdbCluster spec and
// updates the database accordingly. Since the old root host values were stored as the annotations
// inside the statefulset, this method must be called before the statefulset is patched.
func handleRootHostChanges(sc *SyncContext, newRootHost string) error {
	sfset := sc.mysqldSfset
	ndbCluster := sc.ndb

	annotations := sfset.GetAnnotations()
	oldRootHost := annotations[statefulset.RootHost]

	//get the ip address of a MySQL server pod in the MySQL Cluster
	mysqlLabel := ndbCluster.GetCompleteLabels(map[string]string{
		constants.ClusterNodeTypeLabel: sc.mysqldController.GetTypeName(),
	})
	mysqlPodList, _ := sc.podLister.Pods(sc.ndb.Namespace).List(labels.SelectorFromSet(mysqlLabel))
	mysqlServerIp := mysqlPodList[0].Status.PodIP

	if oldRootHost != newRootHost {
		err := mysqlclient.UpdateUser(sc.controllerContext.kubeClientset, ndbCluster, newRootHost, oldRootHost, mysqlServerIp)
		return err
	}
	return nil
}

// createRootUser creates a new "root" user in the database if the user does not exist already. This method is called
// after a new statefulset is created.
func createRootUser(ctx context.Context, sc *SyncContext, newRootHost string) error {

	sfset := sc.mysqldSfset
	ndbCluster := sc.ndb

	annotations := sfset.GetAnnotations()
	rootHost := annotations[statefulset.RootHost]

	// RootHost from sfset differ from the rootHost in config summary.
	// This is an update scenario so just return.
	if rootHost != newRootHost {
		return nil
	}

	rootPasswordSecret := annotations[statefulset.RootPasswordSecret]
	secret, err := sc.controllerContext.kubeClientset.CoreV1().Secrets(ndbCluster.Namespace).Get(ctx, rootPasswordSecret, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("failed to retrieve the MySQL root password secret")
		return err
	}
	password := string(secret.Data[corev1.BasicAuthPasswordKey])
	//get the ip address of a MySQL server pod in the MySQL Cluster
	mysqlLabel := ndbCluster.GetCompleteLabels(map[string]string{
		constants.ClusterNodeTypeLabel: sc.mysqldController.GetTypeName(),
	})
	mysqlPodList, _ := sc.podLister.Pods(sc.ndb.Namespace).List(labels.SelectorFromSet(mysqlLabel))
	mysqlServerIp := mysqlPodList[0].Status.PodIP

	err = mysqlclient.CreateUserIfNotExist(sc.controllerContext.kubeClientset, ndbCluster, password, newRootHost, mysqlServerIp)
	return err
}

// deleteRootUser deletes the "root" user in the database. This method must be called
// before deleting the statefulset.
func deleteRootUser(sc *SyncContext) error {
	sfset := sc.mysqldSfset
	ndbCluster := sc.ndb

	annotations := sfset.GetAnnotations()
	rootHost := annotations[statefulset.RootHost]

	//get the ip address of a MySQL server pod in the MySQL Cluster
	mysqlLabel := ndbCluster.GetCompleteLabels(map[string]string{
		constants.ClusterNodeTypeLabel: sc.mysqldController.GetTypeName(),
	})
	mysqlPodList, _ := sc.podLister.Pods(sc.ndb.Namespace).List(labels.SelectorFromSet(mysqlLabel))
	mysqlServerIp := mysqlPodList[0].Status.PodIP

	err := mysqlclient.DeleteUserIfItExists(sc.controllerContext.kubeClientset, ndbCluster, rootHost, mysqlServerIp)
	return err
}
