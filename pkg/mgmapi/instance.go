// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mgmapi

import (
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"

	"k8s.io/klog"
)

/* Instance represents the local Ndb node instance. All neccessary
information is collected here in order to access the Ndb object
controlling a cluster */
type Instance struct {
	// Namespace is the Kubernetes Namespace in which the instance is running.
	Namespace string
	/* ClusterName is the name of the Cluster to which the instance
	   belongs. Identifies the Ndb CR Object in the namespace */
	ClusterName string
	// ParentName is the name of the StatefulSet to which the instance belongs.
	ParentName string
	// Ordinal is the StatefulSet ordinal of the instances Pod.
	Ordinal int
	// Port is the port on which MySQLDB is listening.
	Port int
	// IP is the IP address of the Kubernetes Pod.
	IP net.IP
}

// NewInstance creates a new Instance. Mostly for testing purposes
func NewInstance(namespace, clusterName, parentName string, ordinal, port int, multiMaster bool) *Instance {
	return &Instance{
		Namespace:   namespace,
		ClusterName: clusterName,
		ParentName:  parentName,
		Ordinal:     ordinal,
		Port:        port,
	}
}

// NewLocalInstance creates a new instance of this structure, with it's name and index
// populated from os.Hostname().
func NewLocalInstance() (*Instance, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	klog.Infof(hostname)
	name, ordinal := GetParentNameAndOrdinal(hostname)
	port, err := strconv.Atoi(os.Getenv("MY_POD_SERVERPORT"))

	return &Instance{
		Namespace:   os.Getenv("MY_POD_NAMESPACE"),
		ClusterName: os.Getenv("MY_NDB_NAME"),
		ParentName:  name,
		Ordinal:     ordinal,
		Port:        port,
		IP:          net.ParseIP(os.Getenv("MY_POD_IP")),
	}, nil
}

// statefulPodRegex is a regular expression that extracts the parent StatefulSet
// and ordinal from StatefulSet Pod's hostname.
var statefulPodRegex = regexp.MustCompile("^(.*)-([0-9]+)$") // (\\.?)(.*)

// GetParentNameAndOrdinal gets the name of a Pod's parent StatefulSet and Pod's
// ordinal from the Pods name (or hostname). If the Pod was not created by a
// StatefulSet, its parent is considered to be empty string, and its ordinal is
// considered to be -1.
func GetParentNameAndOrdinal(name string) (string, int) {
	parent := ""
	ordinal := -1
	s := strings.Split(name, ".")
	if len(s) < 1 {
		return parent, ordinal
	}
	subMatches := statefulPodRegex.FindStringSubmatch(s[0])
	if len(subMatches) < 3 {
		return parent, ordinal
	}
	parent = subMatches[1]
	if i, err := strconv.ParseInt(subMatches[2], 10, 32); err == nil {
		ordinal = int(i)
	}
	return parent, ordinal
}
