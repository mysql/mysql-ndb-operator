// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbtest

import (
	"fmt"
	"github.com/mysql/ndb-operator/e2e-tests/utils/testfiles"
	"github.com/onsi/gomega"
	"time"
)

func runHelmCommand(namespace string, helmArgs []string) (result string) {
	// Append common helm args
	helmArgs = append(helmArgs,
		"--namespace="+namespace,
	)

	if ndbTestSuite.kubeConfig != "" {
		helmArgs = append(helmArgs,
			"--kubeconfig="+ndbTestSuite.kubeConfig,
		)
	}

	// create and run helm cmd
	stdout, _, err := newCmdWithTimeout(
		"helm", "", 10*time.Minute, helmArgs...).run()
	gomega.Expect(err).Should(gomega.Succeed(), fmt.Sprintf("'helm %s' failed", helmArgs[0]))
	return stdout
}

// helmInstall installs the helm chart at chartPath into the given namespace.
func helmInstall(namespace, releaseName, chartPath string) {
	// Build the helm args for helm create command
	helmArgs := []string{
		"install",
		releaseName,
		testfiles.GetAbsPath(chartPath),
		// skip installing CRDs as it will be handled by RunGinkgoSuite
		"--skip-crds",
		// install should be atomic
		"--atomic",
		// wait until the release is ready
		"--wait",
	}

	// Run the command
	result := runHelmCommand(namespace, helmArgs)
	gomega.Expect(result).Should(
		gomega.ContainSubstring("STATUS: deployed"), "'helm install' failed")
}

// helmUninstall uninstalls the given helm release from the given namespace.
func helmUninstall(namespace, releaseName string) {
	// Build the helm args for helm uninstall command
	helmArgs := []string{
		"uninstall",
		releaseName,
	}

	// Run the command
	result := runHelmCommand(namespace, helmArgs)
	gomega.Expect(result).Should(
		gomega.Equal(fmt.Sprintf("release %q uninstalled\n", releaseName)), "'helm uninstall' failed")
}
