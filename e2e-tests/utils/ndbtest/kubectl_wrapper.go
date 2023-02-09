// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbtest

import (
	"strings"
	"time"

	yaml_utils "github.com/mysql/ndb-operator/e2e-tests/utils/yaml"
	"github.com/onsi/gomega"
	klog "k8s.io/klog/v2"
)

const (
	CreateCmd = "create"
	DeleteCmd = "delete"
	ApplyCmd  = "apply"
	GetCmd    = "get"
)

// buildKubectlCmd creates a Cmd for the kubectl command with given arguments
func buildKubectlCmd(namespace, subCommand, data string, extraArgs ...string) *cmdPro {
	// Build the kubectl command
	var kubectlArgs []string

	// Add namespace if not empty
	if namespace != "" {
		kubectlArgs = append(kubectlArgs, "--namespace="+namespace)
	}

	// Append kubeConfig
	if ndbTestSuite.kubeConfig != "" {
		kubectlArgs = append(kubectlArgs, "--kubeconfig="+ndbTestSuite.kubeConfig)
	}

	// Append the subcommand
	kubectlArgs = append(kubectlArgs, subCommand)

	// Append any additional args
	kubectlArgs = append(kubectlArgs, extraArgs...)

	// Append "-f -" arg if any input data is being passed
	if data != "" {
		kubectlArgs = append(kubectlArgs, "-f", "-")
	}

	return newCmdWithTimeout(ndbTestSuite.kubectlPath, data, 1*time.Minute, kubectlArgs...)
}

// RunKubectl is a wrapper around framework.RunKubectlInput that additionally expects no error
func RunKubectl(command, namespace, yamlContent string, extraArgs ...string) string {
	if command != CreateCmd &&
		command != DeleteCmd &&
		command != ApplyCmd &&
		command != GetCmd {
		panic("Unsupported command in getKubectlArgs")
	}

	// Build and run the command
	result, _, err := buildKubectlCmd(namespace, command, yamlContent, extraArgs...).run()
	ExpectNoError(err, "RunKubectl failed with an error")
	klog.V(3).Infof("kubectl executing %s : %s", command, result)
	return result
}

// KubectlGet runs the kubectl get command and returns the result
func KubectlGet(namespace, resourceName string, extraArgs ...string) string {
	klog.V(2).Infof("Running 'kubectl get %s' in namespace %q", resourceName, namespace)
	// Generate args to kubectl get command
	args := append([]string{resourceName}, extraArgs...)
	// Run "kubectl get resourcename <extraArgs>"
	result := RunKubectl(GetCmd, namespace, "", args...)
	return result
}

// CreateObjectsFromYaml extracts the resource identified by the
// Kind, Version, Name and creates that in the k8s cluster
// If the ns string is not empty, the resource will be created
// in the namespace pointed by it
func CreateObjectsFromYaml(path, filename string, k8sObjects []yaml_utils.K8sObject, namespace string) {
	// Apply the yaml to k8s
	kubectlInput := yaml_utils.ExtractObjectsFromYaml(path, filename, k8sObjects, namespace)
	gomega.Expect(kubectlInput).NotTo(gomega.BeEmpty())
	result := RunKubectl(ApplyCmd, namespace, kubectlInput)
	klog.V(3).Infof("kubectl executing %s on some objects from %s: %s", "apply", filename, result)
	gomega.Expect(strings.Count(result, "created")).To(gomega.Equal(len(k8sObjects)))
}

// DeleteObjectsFromYaml extracts the resource identified by the
// Kind, Version, Name and deletes them from the k8s cluster
// If the ns string is not empty, the resource will be deleted
// from the namespace pointed by it
func DeleteObjectsFromYaml(path, filename string, k8sObjects []yaml_utils.K8sObject, namespace string) {
	// Apply the yaml to k8s
	kubectlInput := yaml_utils.ExtractObjectsFromYaml(path, filename, k8sObjects, namespace)
	gomega.Expect(kubectlInput).NotTo(gomega.BeEmpty())
	result := RunKubectl(DeleteCmd, namespace, kubectlInput)
	klog.V(3).Infof("kubectl executing %s on some objects from %s: %s", "delete", filename, result)
	gomega.Expect(strings.Count(result, "deleted")).To(gomega.Equal(len(k8sObjects)))
}
