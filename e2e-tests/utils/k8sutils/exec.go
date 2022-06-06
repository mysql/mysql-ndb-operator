// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package k8sutils

import (
	"bytes"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/klog/v2"
)

// Exec executes the given cmd from the pod with the given podName.
func Exec(client kubernetes.Interface, podName string,
	namespace string, cmd []string) error {

	config, _ := restclient.InClusterConfig()

	// Create a Rest request to the pod using k8s rest client api
	req := client.CoreV1().RESTClient().Post().Resource("pods").Name(podName).Namespace(namespace).SubResource("exec")

	// Exec function is used only to execute MySQL queries. So, input buffer is not required
	option := &v1.PodExecOptions{
		Command: cmd,
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
		TTY:     false,
	}

	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		klog.Infof("error adding to scheme: %s", err)
		return err
	}

	parameterCodec := runtime.NewParameterCodec(scheme)
	req.VersionedParams(
		option,
		parameterCodec,
	)

	// create a SPDY executor from the cluster config
	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		klog.Infof("error executing SPDY request: %s", err)
		return err
	}

	var stdout, stderr bytes.Buffer

	// Connect to the pod, execute the shell commands in the request and gets the output and error streams
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
		Tty:    false,
	})

	if err != nil {
		klog.Infof("error executing commands")
		return err
	}

	klog.Infof("OUTPUT: %s ERR: %s", stdout.String(), stderr.String())
	return nil
}
