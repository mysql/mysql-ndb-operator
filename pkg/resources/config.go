// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers"
)

const (
	// Right now maximum of 144 data nodes are allowed in a cluster
	// So to accommodate scaling up of those nodes, start the api node id from 145
	// TODO: Validate the maximum number of API ids
	apiStartNodeId = 145
)

// NewResourceContextFromConfiguration extracts all relevant information from configuration
// needed for further resource creation (SfSets) or comparison with new incoming ndb.Spec
func NewResourceContextFromConfiguration(configStr string) (*ResourceContext, error) {

	config, err := helpers.ParseString(configStr)
	if err != nil {
		return nil, err
	}

	rc := &ResourceContext{}

	//TODO work with constants
	rc.ConfigHash = helpers.GetValueFromSingleSectionGroup(config, "header", "ConfigHash")

	generationStr := helpers.GetValueFromSingleSectionGroup(config, "system", "ConfigGenerationNumber")
	rc.ConfigGeneration, _ = strconv.ParseInt(generationStr, 10, 64)

	reduncancyLevelStr := helpers.GetValueFromSingleSectionGroup(config, "ndbd default", "NoOfReplicas")
	rl, _ := strconv.ParseUint(reduncancyLevelStr, 10, 32)
	rc.ReduncancyLevel = uint32(rl)

	if rl != 0 {
		noofdatanodes := helpers.GetNumberOfSectionsInSectionGroup(config, "ndbd")
		rc.ConfiguredNodeGroupCount = uint32(noofdatanodes / int(rl))

		noofmgm := helpers.GetNumberOfSectionsInSectionGroup(config, "ndb_mgmd")
		rc.ManagementNodeCount = uint32(noofmgm)
	} else {
		rc.ConfiguredNodeGroupCount = 0
		rc.ManagementNodeCount = 0
	}

	return rc, nil
}

func getMgmdHostname(ndb *v1alpha1.Ndb, count int) string {
	//TODO - get the real domain name
	dnsZone := fmt.Sprintf("%s.svc.cluster.local", ndb.Namespace)
	mgmHostname := fmt.Sprintf("%s-%d.%s.%s", ndb.Name+"-mgmd", count, ndb.GetManagementServiceName(), dnsZone)
	return mgmHostname
}

func getNdbdHostname(ndb *v1alpha1.Ndb, count int) string {
	//TODO - get the real domain name
	dnsZone := fmt.Sprintf("%s.svc.cluster.local", ndb.Namespace)
	mgmHostname := fmt.Sprintf("%s-%d.%s.%s", ndb.Name+"-ndbd", count, ndb.GetDataNodeServiceName(), dnsZone)
	return mgmHostname
}

// GetConfigString produces a new configuration "file" string from the current ndb.Spec
//
// It is important to note that GetConfigString uses the Spec
// in its actual and consistent state and does not rely on any Status field.
func GetConfigString(ndb *v1alpha1.Ndb) (string, error) {

	header := `
	# auto generated config.ini - do not edit
	#
	# ConfigHash={{$confighash}}
	`

	systemSection := `
	[system]
	ConfigGenerationNumber={{$configgeneration}}
	Name={{$clustername}}
	`

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

	// START of generation
	configString := ""

	// header
	hash, err := ndb.CalculateNewConfigHash()
	if err != nil {
		return "", err
	}
	configString += strings.ReplaceAll(header, "{{$confighash}}", hash)
	configString += "\n"

	// system section
	// we use the actual config generation here
	generation := fmt.Sprintf("%d", ndb.GetGeneration())
	syss := systemSection
	syss = strings.ReplaceAll(syss, "{{$configgeneration}}", generation)
	syss = strings.ReplaceAll(syss, "{{$clustername}}", ndb.Name)
	configString += syss + "\n"

	// ndbd default
	noofrepl := fmt.Sprintf("%d", ndb.GetRedundancyLevel())
	configString += strings.ReplaceAll(defaultSections, "{{$noofreplicas}}", noofrepl)
	configString += "\n"

	/*
		TODO - how about hostname/nodeid stability when patching existing config?
	*/
	nodeId := int(1)
	for i := 0; i < int(ndb.GetManagementNodeCount()); i++ {

		ms := mgmdSection
		ms = strings.ReplaceAll(ms, "{{$nodeId}}", strconv.Itoa(nodeId))
		ms = strings.ReplaceAll(ms, "{{$hostname}}", getMgmdHostname(ndb, i))
		ms = strings.ReplaceAll(ms, "{{$datadir}}", constants.DataDir)

		configString += ms
		nodeId++
		configString += "\n"
	}

	// data node sections
	for i := 0; i < int(*ndb.Spec.NodeCount); i++ {

		ns := ndbdSection
		ns = strings.ReplaceAll(ns, "{{$nodeId}}", strconv.Itoa(nodeId))
		ns = strings.ReplaceAll(ns, "{{$hostname}}", getNdbdHostname(ndb, i))
		ns = strings.ReplaceAll(ns, "{{$datadir}}", constants.DataDir)

		configString += ns
		nodeId++
		configString += "\n"
	}
	configString += "\n"

	// mysqld sections
	// at least 1 must be there in order to not fail ndb_mgmd start
	// TODO: Define more api nodes in the beginning so that the user can
	//       scale up the MySQL Server Deployment without restarting the data/mgmd nodes
	mysqlSections := 1
	if ndb.Spec.Mysqld.NodeCount != nil && int(*ndb.Spec.Mysqld.NodeCount) > 0 {
		// we alloc one section more than needed for internal purposes
		mysqlSections = int(*ndb.Spec.Mysqld.NodeCount) + 1
	}

	for i := 0; i < mysqlSections; i++ {
		configString += "[mysqld]\n"
		configString += fmt.Sprintf("NodeId=%d\n", apiStartNodeId+i)
		configString += "\n"
	}

	/* pure estetics - trim whitespace from lines */
	s := strings.Split(configString, "\n")
	configString = ""
	for _, line := range s {
		configString += strings.TrimSpace(line) + "\n"
	}

	//klog.Infof("Config string: \n %s", configString)
	return configString, nil
}
