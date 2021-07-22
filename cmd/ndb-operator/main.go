// Copyright (c) 2020, 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package main

import (
	"flag"
	"time"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/mysql/ndb-operator/config"
	"github.com/mysql/ndb-operator/pkg/controllers"
	clientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
	informers "github.com/mysql/ndb-operator/pkg/generated/informers/externalversions"
	"github.com/mysql/ndb-operator/pkg/helpers"
	"github.com/mysql/ndb-operator/pkg/signals"
)

func main() {
	flag.Parse()

	// set up signal handlers
	stopCh := signals.SetupSignalHandler()

	klog.Infof("Starting ndb-operator with build version %s", config.GetBuildVersion())

	var cfg *restclient.Config
	var err error
	runningInsideK8s := helpers.IsAppRunningInsideK8s()
	if runningInsideK8s {
		// Operator is running inside K8s Pods
		cfg, err = restclient.InClusterConfig()
	} else {
		if config.Kubeconfig == "" && config.MasterURL == "" {
			// Operator is not running inside K8s and kubeconfig/masterURL are not specified.
			klog.Fatal("Ndb operator cannot connect to the Kubernetes Server.\n" +
				"Please specify kubeconfig or masterURL.")
		}
		cfg, err = clientcmd.BuildConfigFromFlags(config.MasterURL, config.Kubeconfig)
	}
	if err != nil {
		klog.Fatalf("Error getting kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	ndbClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building ndb clientset: %s", err.Error())
	}

	k8If := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30)
	ndbOpIf := informers.NewSharedInformerFactory(ndbClient, time.Second*30)

	ctx := controllers.NewControllerContext(kubeClient, ndbClient, runningInsideK8s, config.WatchNamespace)

	controller := controllers.NewController(
		ctx,
		k8If.Apps().V1().StatefulSets(),
		k8If.Apps().V1().Deployments(),
		k8If.Core().V1().Services(),
		k8If.Core().V1().Pods(),
		k8If.Core().V1().ConfigMaps(),
		ndbOpIf.Mysql().V1alpha1().NdbClusters())

	// notice that there is no need to run Start methods in a separate goroutine. (i.e. go kubeInformerFactory.Start(stopCh)
	// Start method is non-blocking and runs all registered informers in a dedicated goroutine.
	k8If.Start(stopCh)
	ndbOpIf.Start(stopCh)

	if err = controller.Run(2, stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}
}

func init() {
	klog.InitFlags(nil)
	config.InitFlags()
}
