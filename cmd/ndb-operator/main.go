// Copyright (c) 2020, 2025, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package main

import (
	"context"
	"flag"
	"time"

	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	klog "k8s.io/klog/v2"

	"github.com/mysql/ndb-operator/config"
	"github.com/mysql/ndb-operator/pkg/controllers"
	clientset "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned"
	ndbinformers "github.com/mysql/ndb-operator/pkg/generated/informers/externalversions"
	"github.com/mysql/ndb-operator/pkg/helpers"
	"github.com/mysql/ndb-operator/pkg/signals"
)

func main() {
	flag.Parse()
	config.ValidateFlags()

	// set up signal handlers
	ctx := signals.SetupSignalHandler(context.Background())

	klog.Infof("Starting ndb-operator with build version %s", config.GetBuildVersion())

	// K8s client configuration
	var cfg *restclient.Config
	var err error

	runningInsideK8s := helpers.IsAppRunningInsideK8s()
	if runningInsideK8s {
		// Operator is running inside K8s Pods
		cfg, err = restclient.InClusterConfig()

		// First, we check if the namespace to watch is available from the environment.
		// Only if it could not be found, the command flags will be used.
		watchNamespace := helpers.GetWatchNamespaceFromEnvironment()
		if watchNamespace != "" {
			// Operator is in Single Namespace install mode
			klog.Infof("Getting watch namespace from environment: %s", watchNamespace)
			config.ClusterScoped = false
			config.WatchNamespace = watchNamespace
		} else {
			klog.Info("No namespace to watch found in environment. Using flags.")
			if !config.ClusterScoped && config.WatchNamespace == "" {
				// Operator is namespace-scoped.
				// If no namespace is specified
				// Use the namespace it is deployed in to watch for changes.
				config.WatchNamespace, err = helpers.GetCurrentNamespace()
				if err != nil {
					klog.Fatalf("Could not get current namespace : %s", err)
				}
			}
		}
	} else {
		// Operator is running outside K8s cluster
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

	if config.ClusterScoped {
		klog.Info("Running NDB Operator with cluster-scope")
	} else {
		klog.Info("Running NDB Operator with namespace-scope")
		klog.Infof("Watching for changes in namespace %q", config.WatchNamespace)
	}

	if config.EnableSecurityContext {
		klog.Info("Pods launched by NDB Operator will have their security context enabled.")
		klog.Info("Please make sure your persistent volumes have the correct permissions for data access")
		if config.UsePlatformAssignedIDs {
			klog.Info("The NDB Operator will deploy the NDB process with the UID and GID values assigned automatically by the platform.")
		} else {
			klog.Infof("The NDB Operator will deploy the NDB processes with UID: %d, GID: %d, FSGroup: %d",
				config.RunAsUser, config.RunAsGroup, config.FSGroup)
		}
	} else {
		klog.Info("Pods launched by NDB Operator will not have their security context enabled")
	}

	// Create a SharedInformerFactory and limit it to config.WatchNamespace.
	// For cluster-scoped mode, config.WatchNamespace will be empty
	// and the SharedInformer will not be limited to any namespace.
	k8If := kubeinformers.NewSharedInformerFactoryWithOptions(
		kubeClient, time.Second*30, kubeinformers.WithNamespace(config.WatchNamespace))
	ndbIf := ndbinformers.NewSharedInformerFactoryWithOptions(
		ndbClient, time.Second*30, ndbinformers.WithNamespace(config.WatchNamespace))

	controller := controllers.NewController(kubeClient, ndbClient, k8If, ndbIf)

	// notice that there is no need to run Start methods in a separate goroutine. (i.e. go kubeInformerFactory.Start(stopCh)
	// Start method is non-blocking and runs all registered informers in a dedicated goroutine.
	k8If.Start(ctx.Done())
	ndbIf.Start(ctx.Done())

	if err = controller.Run(ctx, 2); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}
}

func init() {
	klog.InitFlags(nil)
	config.InitFlags()
}
