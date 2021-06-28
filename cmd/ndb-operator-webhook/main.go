package main

import (
	"github.com/mysql/ndb-operator/config"
	"github.com/mysql/ndb-operator/pkg/webhook"
	"k8s.io/klog"
)

func main() {
	klog.Infof("Starting ndb-operator webhook with version %s",
		config.GetBuildVersion())
	webhook.Run()
}
