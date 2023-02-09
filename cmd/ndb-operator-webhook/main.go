// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package main

import (
	"github.com/mysql/ndb-operator/config"
	"github.com/mysql/ndb-operator/pkg/webhook"
	klog "k8s.io/klog/v2"
)

func main() {
	klog.Infof("Starting ndb-operator webhook with version %s",
		config.GetBuildVersion())
	webhook.Run()
}
