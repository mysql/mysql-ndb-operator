// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbtest

import (
	"context"
	"fmt"
	"regexp"
	"time"

	crdutils "github.com/mysql/ndb-operator/e2e-tests/utils/crd"
	ndbclient "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
	ginkgo "github.com/onsi/ginkgo/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	klog "k8s.io/klog/v2"
)

const (
	ndbOperatorHelmChart   = "deploy/charts/ndb-operator"
	ndbOperatorReleaseName = "ndb-operator-rel"
)

// WaitForNdbOperatorReady waits for the NDB Operator webhook server to become ready
func WaitForNdbOperatorReady(ctx context.Context, ndbClientset ndbclient.Interface, namespace string) {
	nc := crdutils.NewTestNdbCrd(namespace, "test-webhook-ready", 1, 1, 1)
	err := wait.PollImmediate(500*time.Millisecond, 5*time.Minute, func() (done bool, err error) {
		// The Webhook server is ready if a dry run attempt to create a new NdbCluster object succeeds
		_, err = ndbClientset.MysqlV1().NdbClusters(namespace).Create(ctx, nc, metav1.CreateOptions{
			DryRun: []string{metav1.DryRunAll},
		})

		if err == nil {
			// The webhook server is ready
			return true, nil
		}

		// If the webhook server is not ready, create will return a 'failed calling webhook' error
		expectedErrRegex := regexp.MustCompile("Internal error occurred: failed calling webhook .*")
		if expectedErrRegex.MatchString(err.Error()) {
			// The webhook server is not ready yet
			klog.Infof("create --dry-run failed : %s", err)
			return false, nil
		}

		// An unexpected error occurred during the dry run
		return false, err
	})
	ExpectNoError(err, "failed waiting for the NdbCluster admission controller to get ready")
}

// installNdbOperator creates all the K8s resources and RBACs required
// by the Ndb Operator and deploys the Ndb Operator and webhook server
// using helm.
func installNdbOperator(tc *TestContext) {
	namespace := tc.Namespace()
	desc := fmt.Sprintf(
		"Installing NDB Operator, web hook server and related resources in namespace %q", namespace)
	ginkgo.By(desc)
	HelmInstall(namespace, ndbOperatorReleaseName, ndbOperatorHelmChart, false)

	// Operator was successfully installed
	// Set the ndbOperatorInstalled bool flag to record this.
	tc.ndbOperatorInstalled = true
	WaitForNdbOperatorReady(tc.ctx, tc.ndbClientset, namespace)
}

// uninstallNdbOperator deletes the Ndb Operator and webhook server
// deployments and all the K8s resources and RBACs created for the
// Ndb Operator using helm.
func uninstallNdbOperator(tc *TestContext) {
	if !tc.ndbOperatorInstalled {
		klog.Info("Ndb Operator was not installed")
		return
	}

	namespace := tc.Namespace()
	desc := fmt.Sprintf(
		"Uninstalling NDB Operator, web hook server and related resources from namespace %q", namespace)
	ginkgo.By(desc)
	HelmUninstall(namespace, ndbOperatorReleaseName)
}
