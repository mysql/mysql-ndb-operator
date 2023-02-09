// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"strconv"

	"github.com/mysql/ndb-operator/pkg/resources"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	policylisterv1 "k8s.io/client-go/listers/policy/v1"
	klog "k8s.io/klog/v2"
)

func ServerSupportsV1Policy(client kubernetes.Interface) bool {
	info, err := client.Discovery().ServerVersion()
	if err != nil {
		klog.Warning(
			"Cannot use PodDisruptionBudgets as operator is unable to detect connected K8s server version :", err)
		return false
	}

	majorVersion, _ := strconv.Atoi(info.Major)
	minorVersion, _ := strconv.Atoi(info.Minor)
	if majorVersion == 1 && minorVersion < 25 && minorVersion >= 19 {
		return true
	}

	klog.Warningf(
		"Cannot use PodDisruptionBudgets as K8s Server version %q doesn't support v1/policy", info.String())
	return false
}

type PodDisruptionBudgetControlInterface interface {
	EnsurePodDisruptionBudget(
		ctx context.Context, sc *SyncContext, nodeType string) (existed bool, err error)
}

type podDisruptionBudgetImpl struct {
	k8sClient kubernetes.Interface
	pdbLister policylisterv1.PodDisruptionBudgetLister
}

// NewPodDisruptionBudgetControl creates a new PodDisruptionBudgetControlInterface
func newPodDisruptionBudgetControl(
	client kubernetes.Interface,
	pdbLister policylisterv1.PodDisruptionBudgetLister) PodDisruptionBudgetControlInterface {
	return &podDisruptionBudgetImpl{
		k8sClient: client,
		pdbLister: pdbLister,
	}
}

// EnsurePodDisruptionBudget creates a new PodDisruptionBudget
// for the given node type if one doesn't exist yet.
func (pdbi *podDisruptionBudgetImpl) EnsurePodDisruptionBudget(
	ctx context.Context, sc *SyncContext, nodeType string) (existed bool, err error) {

	// Check if the PDB exists already
	nc := sc.ndb
	pdbName := nc.GetPodDisruptionBudgetName(nodeType)
	pdb, err := pdbi.pdbLister.PodDisruptionBudgets(nc.Namespace).Get(pdbName)

	if err == nil {
		// PDB exists. Verify that it is owned by the NdbCluster resource.
		if err = sc.isOwnedByNdbCluster(pdb); err != nil {
			// PDB is not owned by NdbCluster resource
			return false, err
		}

		return true, nil
	}

	if !apierrors.IsNotFound(err) {
		// Error retrieving PDB
		klog.Errorf("Failed to retrieve PDB \"%s/%s\" : %s",
			nc.Namespace, pdbName, err)
	}

	// PDB doesn't exist yet. Create it.
	klog.Infof("Creating a PodDisruptionBudget for node type %q : \"%s/%s\"",
		nodeType, nc.Namespace, pdbName)
	pdb = resources.NewPodDisruptionBudget(nc, nodeType)
	pdbInterface := sc.kubeClientset().PolicyV1().PodDisruptionBudgets(sc.ndb.Namespace)
	_, err = pdbInterface.Create(ctx, pdb, metav1.CreateOptions{})

	if err != nil && !apierrors.IsAlreadyExists(err) {
		// Error creating PDB.
		// Ignore AlreadyExists error as the cache read might be outdated.
		return false, err
	}

	// Successfully created a PDB for the given node type
	return false, nil
}
