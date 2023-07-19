// Copyright (c) 2020, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package ndbconfig

import (
	"bytes"
	"net"
	"text/template"

	klog "k8s.io/klog/v2"

	v1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/constants"
)

// Operator sets NoOfReplicas and ServerPort in the default ndbd section
const numOfOperatorSetConfigs = 2

// MySQL Cluster config template
var mgmtConfigTmpl = `{{- /* Template to generate management config ini */ -}}
# Auto generated config.ini - DO NOT EDIT

[system]
ConfigGenerationNumber={{GetConfigVersion}}
Name={{.Name}}

{{if .Spec.ManagementNode}}{{if .Spec.ManagementNode.Config}}[ndb_mgmd default]{{end}}
{{- range $configKey, $configValue := .Spec.ManagementNode.Config }}
{{$configKey}}={{$configValue}}
{{- end}}{{end}}

[ndbd default]
{{/* update numOfOperatorSetConfigs if a new parameter is added here */ -}}
NoOfReplicas={{.Spec.RedundancyLevel}}
# Use a fixed ServerPort for all data nodes
ServerPort=1186
{{- if .Spec.TDESecretName }}
EncryptedFileSystem=1
{{ end }}
{{- range $configKey, $configValue := .Spec.DataNode.Config }}
{{$configKey}}={{$configValue}}
{{- end}}

[tcp default]
AllowUnresolvedHostnames=1

{{$hostnameSuffix := GetHostnameSuffix -}}
{{range $idx, $nodeId := GetNodeIds NdbNodeTypeMgmd -}}
[ndb_mgmd]
NodeId={{$nodeId}}
Hostname={{$.Name}}-{{NdbNodeTypeMgmd}}-{{$idx}}.{{$.GetServiceName NdbNodeTypeMgmd}}.{{$hostnameSuffix}}
DataDir={{GetDataDir}}

{{end -}}
{{range $idx, $nodeId := GetNodeIds NdbNodeTypeNdbmtd -}}
[ndbd]
NodeId={{$nodeId}}
Hostname={{$.Name}}-{{NdbNodeTypeNdbmtd}}-{{$idx}}.{{$.GetServiceName NdbNodeTypeNdbmtd}}.{{$hostnameSuffix}}
DataDir={{GetDataDir}}
{{if IsNewDataNode $nodeId -}}
NodeGroup=65536
{{end}}
{{end -}}
# Dedicated API section to be used by NDB Operator
[api]
NodeId={{NdbOperatorDedicatedAPINodeId}}
Dedicated=1

# MySQLD sections to be used exclusively by MySQL Servers
{{range $nodeId, $podIdx := GetMySQLServerNodeIds -}}
[mysqld]
NodeId={{$nodeId}}
Hostname={{$.Name}}-{{NdbNodeTypeMySQLD}}-{{$podIdx}}.{{$.GetServiceName NdbNodeTypeMySQLD}}.{{$hostnameSuffix}}

{{end -}}
# API sections to be used by generic NDBAPI applications
{{range $nodeId := GetNodeIds NdbNodeTypeAPI -}}
[api]
NodeId={{$nodeId}}

{{end -}}
`

// GetConfigString generates a new configuration for the
// MySQL Cluster from the given ndb resources Spec.
//
// It is important to note that GetConfigString uses the Spec in its
// actual and consistent state and does not rely on any Status field.
func GetConfigString(ndb *v1.NdbCluster, oldConfigSummary *ConfigSummary) (string, error) {

	var (
		// Variables that keep track of the first free mgmd/data node id and api nodeId
		ndbdMgmdStartNodeId = 1
		apiStartNodeId      = constants.NdbNodeTypeAPIStartNodeId

		// newDataNodeStartId tracks the starting nodeId of the new
		// data nodes during an online add data node scenario
		newDataNodeStartId = 0
	)

	if oldConfigSummary != nil && oldConfigSummary.NumOfDataNodes < ndb.Spec.DataNode.NodeCount {
		// Data Nodes are being added to the configuration.
		newDataNodeStartId = int(ndb.GetManagementNodeCount() + oldConfigSummary.NumOfDataNodes + 1)
	}

	tmpl := template.New("config.ini")
	tmpl.Funcs(template.FuncMap{
		// GetNodeIds returns an array of node ids for the given node type
		"GetNodeIds": func(nodeType string) []int {
			var startNodeId *int
			var numberOfNodes int32
			switch nodeType {
			case constants.NdbNodeTypeMgmd:
				startNodeId = &ndbdMgmdStartNodeId
				numberOfNodes = ndb.GetManagementNodeCount()
			case constants.NdbNodeTypeNdbmtd:
				startNodeId = &ndbdMgmdStartNodeId
				numberOfNodes = ndb.Spec.DataNode.NodeCount
			case constants.NdbNodeTypeMySQLD:
				startNodeId = &apiStartNodeId
				numberOfNodes = GetNumOfSectionsRequiredForMySQLServers(ndb)
			case constants.NdbNodeTypeAPI:
				startNodeId = &apiStartNodeId
				numberOfNodes = ndb.Spec.FreeAPISlots
			default:
				panic("Unrecognised node type")
			}
			// generate nodeIds based on start node id and number of nodes
			nodeIds := make([]int, numberOfNodes)
			for i := range nodeIds {
				nodeIds[i] = *startNodeId
				*startNodeId++
			}
			return nodeIds
		},
		"GetMySQLServerNodeIds": func() map[int]int {
			startNodeId := &apiStartNodeId
			numberOfNodeIds := GetNumOfSectionsRequiredForMySQLServers(ndb)
			nodeIdToPodIdx := make(map[int]int)
			ndbConnectionPoolSize := ndb.GetMySQLServerConnectionPoolSize()
			podIdx := 0
			for i := 0; int32(i) < numberOfNodeIds; i = i + int(ndbConnectionPoolSize) {
				for j := 0; j < int(ndbConnectionPoolSize); j++ {
					nodeIdToPodIdx[*startNodeId] = podIdx
					*startNodeId++
				}
				podIdx++
			}

			return nodeIdToPodIdx
		},
		"GetDataDir": func() string { return constants.DataDir + "/data" },
		"IsNewDataNode": func(nodeId int) bool {
			return newDataNodeStartId != 0 && nodeId >= newDataNodeStartId
		},
		"GetConfigVersion": func() int32 {
			if oldConfigSummary == nil {
				// First version of the management config based on newly added NdbCluster spec.
				return 1
			} else {
				// Bump up the config version for every change.
				return oldConfigSummary.MySQLClusterConfigVersion + 1
			}
		},
		"GetHostnameSuffix": func() string {
			// If the K8s Cluster domain can be found, generate the hostname suffix of form :
			// '<namespace>.svc.<k8s-cluster-domain>' or else, simply use the namespace as the suffix.
			// Deduce K8s cluster domain suffix by looking up the kubernetes server's CNAME.
			if k8sCname, err := net.LookupCNAME("kubernetes.default.svc"); err != nil {
				klog.Warning("K8s Cluster domain lookup failed :", err.Error())
				klog.Warning("Using partial subdomain as Hostnames in management configuration")
				return ndb.Namespace
			} else {
				// Found the FQDN of form "kubernetes.default.svc.<k8s-cluster-domain>."
				// Extract the required parts and append them to the suffix
				return ndb.Namespace + k8sCname[len("kubernetes.default"):len(k8sCname)-1]
			}
		},
		"NdbNodeTypeMgmd":               func() string { return constants.NdbNodeTypeMgmd },
		"NdbNodeTypeNdbmtd":             func() string { return constants.NdbNodeTypeNdbmtd },
		"NdbNodeTypeMySQLD":             func() string { return constants.NdbNodeTypeMySQLD },
		"NdbNodeTypeAPI":                func() string { return constants.NdbNodeTypeAPI },
		"NdbOperatorDedicatedAPINodeId": func() int { return constants.NdbOperatorDedicatedAPINodeId },
	})

	if _, err := tmpl.Parse(mgmtConfigTmpl); err != nil {
		// panic to discover any parsing errors during development
		panic("Failed to parse mgmt config template : " + err.Error())
	}

	var configIni bytes.Buffer
	if err := tmpl.Execute(&configIni, ndb); err != nil {
		return "", err
	}

	return configIni.String(), nil
}

// MySQL Server config (my.cnf) template
var myCnfTemplate = `{{- /* Template to generate my.cnf config ini */ -}}
# Auto generated config.ini - DO NOT EDIT
# ConfigVersion={{GetConfigVersion}}

{{.GetMySQLCnf}}
`

// GetMySQLConfigString returns the MySQL Server config(my.cnf)
// to be used by the MySQL Server StatefulSet.
func GetMySQLConfigString(nc *v1.NdbCluster, oldConfigSummary *ConfigSummary) (string, error) {

	if nc.GetMySQLCnf() == "" {
		return "", nil
	}

	tmpl := template.New("my.cnf")
	tmpl.Funcs(template.FuncMap{
		"GetConfigVersion": func() int32 {
			if oldConfigSummary == nil {
				// First version of the my.cnf config based on newly added NdbCluster spec.
				return 1
			} else {
				// Bump up the config version for every change.
				return oldConfigSummary.MySQLServerConfigVersion + 1
			}
		},
	})

	if _, err := tmpl.Parse(myCnfTemplate); err != nil {
		// panic to discover any parsing errors during development
		panic("Failed to parse my.cnf config template")
	}

	var myCnf bytes.Buffer
	if err := tmpl.Execute(&myCnf, nc); err != nil {
		return "", err
	}

	return myCnf.String(), nil
}
