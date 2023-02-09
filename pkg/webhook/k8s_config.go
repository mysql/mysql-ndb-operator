// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package webhook

import (
	"flag"

	k8s "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	klog "k8s.io/klog/v2"
)

// command line arguments related to k8s
var config struct {
	// K8s service
	serviceName string
	// K8s config for out of cluster run
	masterURL, kubeconfig string
}

func mandatoryParam(param string, value string) {
	if len(value) == 0 {
		flag.Usage()
		klog.Fatalf("Required parameter '-%s' is missing", param)
	}
}

func validateCommandLineArgs() {
	// Check required arguments
	mandatoryParam("service", config.serviceName)
}

func init() {
	// klog arguments
	klog.InitFlags(nil)

	// argument to get the k8s service name
	flag.StringVar(&config.serviceName, "service", "",
		"Name of the K8s service that will be mapped to the webhook server (Required)")

	// out-of-cluster kubeconfig/masterURL arguments
	flag.StringVar(&config.masterURL, "master", "",
		"The address of the Kubernetes API server. "+
			"Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&config.kubeconfig, "kubeconfig", "",
		"Path to a kubeconfig. Only required if out-of-cluster.")
}

// The clientset to the k8s cluster. Do not use this directly,
// rather use getK8sClientset to ensure that this is created properly
var _clientset *k8s.Clientset

// getK8sClientset creates a clientset using the given config
func getK8sClientset() *k8s.Clientset {
	if _clientset != nil {
		return _clientset
	}
	// Create the clientset and return
	var cfg *restclient.Config
	var err error
	if len(config.masterURL) == 0 && len(config.kubeconfig) == 0 {
		cfg, err = restclient.InClusterConfig()
	} else {
		cfg, err = clientcmd.BuildConfigFromFlags(config.masterURL, config.kubeconfig)
	}
	if err != nil {
		klog.Error("Error building kubeconfig: ", err)
		return nil
	}

	_clientset, err = k8s.NewForConfig(cfg)
	if err != nil {
		klog.Error("Error building kubernetes clientset: ", err)
		return nil
	}

	return _clientset
}
