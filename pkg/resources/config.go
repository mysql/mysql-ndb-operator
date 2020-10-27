// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ocklin/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/ocklin/ndb-operator/pkg/constants"
)

func getMgmdHostname(ndb *v1alpha1.Ndb, count int) string {
	dnsZone := fmt.Sprintf("%s.svc.cluster.local", ndb.Namespace)
	mgmHostname := fmt.Sprintf("%s-%d.%s.%s", ndb.Name+"-mgmd", count, ndb.Name, dnsZone)
	return mgmHostname
}

func getNdbdHostname(ndb *v1alpha1.Ndb, count int) string {
	dnsZone := fmt.Sprintf("%s.svc.cluster.local", ndb.Namespace)
	mgmHostname := fmt.Sprintf("%s-%d.%s.%s", ndb.Name+"-ndbd", count, ndb.Name, dnsZone)
	return mgmHostname
}

func GetConfigString(ndb *v1alpha1.Ndb) (string, error) {

	defaultSections := `
  [ndbd default]
  NoOfReplicas={{$noofreplicas}}
  DataMemory=80M

  [tcp default]
  AllowUnresolvedHostnames=1`

	mgmdSection := `	
  [ndb_mgmd]
  NodeId={{$nodeId}}
  Hostname={{$hostname}}
  DataDir={{$datadir}}`

	ndbdSection := `
  [ndbd]
  NodeId={{$nodeId}}
  Hostname={{$hostname}}
  DataDir={{$datadir}}
  ServerPort=1186`

	noofrepl := fmt.Sprintf("%d", *ndb.Spec.Ndbd.NoOfReplicas)
	//noofrepl := strconv.Itoa(*ndb.Spec.Ndbd.NoOfReplicas)
	configString := strings.ReplaceAll(defaultSections, "{{$noofreplicas}}", noofrepl)
	configString += "\n"

	/*
		TODO - how about hostname/nodeid stability when patching existing config?
	*/
	nodeId := int(1)
	for i := 0; i < int(*ndb.Spec.Mgmd.NodeCount); i++ {

		ms := mgmdSection
		ms = strings.ReplaceAll(ms, "{{$nodeId}}", strconv.Itoa(nodeId))
		ms = strings.ReplaceAll(ms, "{{$hostname}}", getMgmdHostname(ndb, i))
		ms = strings.ReplaceAll(ms, "{{$datadir}}", constants.DataDir)

		configString += ms
		nodeId++
		configString += "\n"
	}

	// data node sections
	for i := 0; i < int(*ndb.Spec.Ndbd.NodeCount); i++ {

		ns := ndbdSection
		ns = strings.ReplaceAll(ns, "{{$nodeId}}", strconv.Itoa(nodeId))
		ns = strings.ReplaceAll(ns, "{{$hostname}}", getNdbdHostname(ndb, i))
		ns = strings.ReplaceAll(ns, "{{$datadir}}", constants.DataDir)

		configString += ns
		nodeId++
		configString += "\n"
	}

	// mysqld sections
	// at least 1 must be there in order to not fail ndb_mgmd start
	configString += "[mysqld]\n"
	configString += "[mysqld]\n"

	/* pure estetics - trim whitespace from lines */
	s := strings.Split(configString, "\n")
	configString = ""
	for _, line := range s {
		configString += strings.TrimSpace(line) + "\n"
	}
	return configString, nil
}
