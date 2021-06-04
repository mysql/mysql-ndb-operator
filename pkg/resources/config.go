// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package resources

import (
	"bytes"
	"strconv"
	"text/template"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers"
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

// MySQL Cluster config template
var mgmtConfigTmpl = `{{- /* Template to generate management config ini */ -}}
# auto generated config.ini - do not edit
#
# ConfigHash={{.CalculateNewConfigHash}}

[system]
ConfigGenerationNumber={{.GetGeneration}}
Name={{.Name}}

[ndbd default]
NoOfReplicas={{.GetRedundancyLevel}}
DataMemory=80M
# Use a fixed ServerPort for all data nodes
ServerPort=1186

[tcp default]
AllowUnresolvedHostnames=1

{{range $idx, $nodeId := GetNodeIds "mgmd" -}}
[ndb_mgmd]
NodeId={{$nodeId}}
{{/*TODO - get and use the real domain name instead of .svc.cluster.local */ -}}
Hostname={{$.Name}}-mgmd-{{$idx}}.{{$.GetServiceName "mgmd"}}.{{$.Namespace}}.svc.cluster.local
DataDir={{GetDataDir}}

{{end -}}
{{range $idx, $nodeId := GetNodeIds "ndbd" -}}
[ndbd]
NodeId={{$nodeId}}
{{/*TODO - get and use the real domain name instead of .svc.cluster.local */ -}}
Hostname={{$.Name}}-ndbd-{{$idx}}.{{$.GetServiceName "ndbd"}}.{{$.Namespace}}.svc.cluster.local
DataDir={{GetDataDir}}

{{end -}}
{{range $nodeId := GetNodeIds "api" -}}
[mysqld]
NodeId={{$nodeId}}

{{end -}}
`

// GetConfigString generates a new configuration for the
// MySQL Cluster from the given ndb resources Spec.
//
// It is important to note that GetConfigString uses the Spec in its
// actual and consistent state and does not rely on any Status field.
func GetConfigString(ndb *v1alpha1.Ndb) (string, error) {

	var (
		// Variable that keeps track of the first free data node, mgmd node ids
		ndbdMgmdStartNodeId = 1
		// Right now maximum of 144 data nodes are allowed in a cluster
		// So to accommodate scaling up of those nodes, start the api node id from 145
		// TODO: Validate the maximum number of API ids
		apiStartNodeId = 145
	)

	tmpl := template.New("config.ini")
	tmpl.Funcs(template.FuncMap{
		// GetNodeIds returns an array of node ids for the given node type
		"GetNodeIds": func(nodeType string) []int {
			var startNodeId *int
			var numberOfNodes int32
			switch nodeType {
			case "mgmd":
				startNodeId = &ndbdMgmdStartNodeId
				numberOfNodes = int32(ndb.GetManagementNodeCount())
				break
			case "ndbd":
				startNodeId = &ndbdMgmdStartNodeId
				numberOfNodes = *ndb.Spec.NodeCount
				break
			case "api":
				startNodeId = &apiStartNodeId
				// number of api slots = slots required for mysql server + 1
				numberOfNodes = ndb.GetMySQLServerNodeCount()
				numberOfNodes++
				break
			default:
				panic("Unrecognised node type")
			}
			// generate nodeis based on start node id and number of nodes
			nodeIds := make([]int, numberOfNodes)
			for i := range nodeIds {
				nodeIds[i] = *startNodeId
				*startNodeId++
			}
			return nodeIds
		},
		"GetDataDir": func() string { return constants.DataDir },
	})

	if _, err := tmpl.Parse(mgmtConfigTmpl); err != nil {
		// panic to discover any parsing errors during development
		panic("Failed to parse mgmt config template")
	}

	var configIni bytes.Buffer
	if err := tmpl.Execute(&configIni, ndb); err != nil {
		return "", err
	}

	return configIni.String(), nil
}
