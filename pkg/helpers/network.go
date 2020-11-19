// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package helpers

import (
	"net"
	"strings"
)

// GetClusterDomain returns Kubernetes cluster domain, default to "cluster.local"
func getClusterDomain() string {
	apiSvc := "kubernetes.default.svc"

	cname, err := net.LookupCNAME(apiSvc)
	if err != nil {
		defaultClusterDomain := "cluster.local"
		return defaultClusterDomain
	}

	clusterDomain := strings.TrimPrefix(cname, apiSvc)
	clusterDomain = strings.TrimSuffix(clusterDomain, ".")

	return clusterDomain
}
