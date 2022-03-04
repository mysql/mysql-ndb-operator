// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package v1alpha1

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/ndbconfig/configparser"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// List of all config parameters that are not allowed in
// .Spec.DataNodeConfig. and any additional details to be appended to the error.
var disallowedConfigParams = map[string]string{
	// NoOfReplicas is already set via .spec.redundancyLevel
	"noofreplicas": "Specify it via .spec.redundancyLevel.", // NoOfReplicas
	// Disallow any parameter that specifies machine/port
	// configurations as that will be taken care by the operator.
	"nodeid":            "", // NodeId
	"hostname":          "", // HostName
	"serverport":        "", // ServerPort
	"executeoncomputer": "", // ExecuteOnComputer
	"nodegroup":         "", // NodeGroup
	// Disallow DataDir config as that will be handled by the operator
	"datadir": "", // DataDir
}

// HasValidSpec validates the spec of the NdbCluster object
func (nc *NdbCluster) HasValidSpec() (bool, field.ErrorList) {
	spec := nc.Spec

	var errList field.ErrorList
	specPath := field.NewPath("spec")
	mysqldPath := specPath.Child("mysqld")

	dataNodeCount := spec.NodeCount
	mysqlServerCount := nc.GetMySQLServerNodeCount()
	managementNodeCount := nc.GetManagementNodeCount()
	numOfFreeApiSlots := spec.FreeAPISlots

	// check if number of data nodes is a multiple of redundancy
	if math.Mod(float64(dataNodeCount), float64(spec.RedundancyLevel)) != 0 {
		msg := fmt.Sprintf(
			"spec.nodeCount should be a multiple of the spec.redundancyLevel(=%d)", spec.RedundancyLevel)
		errList = append(errList, field.Invalid(specPath.Child("nodeCount"), dataNodeCount, msg))
	}

	// check if total number of nodes are not more than the allowed maximum
	total := managementNodeCount + dataNodeCount + mysqlServerCount + numOfFreeApiSlots
	if total > constants.MaxNumberOfNodes {
		invalidValue := fmt.Sprintf(
			"%d (= %d management, %d data, %d mysql nodes and %d free API nodes)",
			total, managementNodeCount, dataNodeCount, mysqlServerCount, numOfFreeApiSlots)
		msg := fmt.Sprintf(
			"Total number of MySQL Cluster nodes should not exceed the allowed maximum of %d", constants.MaxNumberOfNodes)
		errList = append(errList, field.Invalid(field.NewPath("Total Nodes"), invalidValue, msg))
	}

	// check if there are any disallowed config params in dataNodeConfig.
	if nc.Spec.DataNodeConfig != nil {
		dataNodeConfigPath := specPath.Child("dataNodeConfig")
		for configKey := range nc.Spec.DataNodeConfig {
			if details, exists := disallowedConfigParams[strings.ToLower(configKey)]; exists {
				msg := fmt.Sprintf("config param %q is not allowed in .spec.dataNodeConfig. ", configKey)
				if details == "" {
					msg += "It will be configured automatically by the Ndb Operator based on the spec."
				} else {
					msg += details
				}
				// errList = append(errList, field.Invalid(specPath.Child("dataNodeConfig"), configKey, msg))
				errList = append(errList, field.Forbidden(dataNodeConfigPath.Child(configKey), msg))
			}
		}
	}

	// check if the MySQL root password secret name has the expected format
	var rootPasswordSecret string
	if nc.Spec.Mysqld != nil {
		rootPasswordSecret = nc.Spec.Mysqld.RootPasswordSecretName
	}
	if rootPasswordSecret != "" {
		errs := validation.IsDNS1123Subdomain(rootPasswordSecret)
		// append errors, if any, to errList
		for _, err := range errs {
			errList = append(errList,
				field.Invalid(mysqldPath.Child("rootPasswordSecretName"), rootPasswordSecret, err))
		}
	}

	// check if any passed my.cnf has proper format
	myCnfString := nc.GetMySQLCnf()
	if len(myCnfString) > 0 {
		myCnf, err := configparser.ParseString(myCnfString)
		if err != nil {
			// error parsing the cnf
			errList = append(errList,
				field.Invalid(mysqldPath.Child("myCnf"), myCnfString, err.Error()))
		} else {
			// accept only one mysqld section in the cnf
			if len(myCnf) != 1 ||
				myCnf.GetNumberOfSections("mysqld") != 1 {
				errList = append(errList,
					field.Invalid(mysqldPath.Child("myCnf"),
						myCnfString, "spec.mysqld.myCnf can have only one mysqld section"))
			}
		}
	}

	return errList == nil, errList
}

func (nc *NdbCluster) IsValidSpecUpdate(newNc *NdbCluster) (bool, field.ErrorList) {

	var errList field.ErrorList
	specPath := field.NewPath("spec")
	mysqldPath := specPath.Child("mysqld")

	if nc.Spec.RedundancyLevel == 1 {
		// MySQL Cluster replica = 1 => updating MySQL config via
		// rolling restart is not possible. Disallow any spec update.
		errList = append(errList,
			field.InternalError(specPath,
				errors.New("operator cannot handle any spec update to a MySQL Cluster whose replica is 1")))
		return false, errList
	}

	// Do not allow updating Spec.NodeCount and Spec.RedundancyLevel
	if nc.Spec.NodeCount != newNc.Spec.NodeCount {
		var msg string
		if nc.Spec.NodeCount < newNc.Spec.NodeCount {
			msg = "Online add node is not supported by the operator yet"
		} else {
			msg = "spec.NodeCount cannot be reduced once MySQL Cluster has been started"
		}
		errList = append(errList,
			field.Invalid(specPath.Child("nodeCount"), newNc.Spec.NodeCount, msg))
	}

	if nc.Spec.RedundancyLevel != newNc.Spec.RedundancyLevel {
		errList = append(errList,
			field.Invalid(specPath.Child("redundancyLevel"), newNc.Spec.RedundancyLevel,
				"spec.redundancyLevel cannot be updated once MySQL Cluster has been started"))
	}

	// Do not allow updating Spec.Mysqld.RootHost
	if nc.Spec.Mysqld != nil &&
		newNc.Spec.Mysqld != nil &&
		(nc.Spec.Mysqld.RootHost != newNc.Spec.Mysqld.RootHost) {
		errList = append(errList,
			field.Invalid(mysqldPath.Child("rootHost"), newNc.Spec.Mysqld.RootHost,
				"spec.mysqld.rootHost cannot be updated once MySQL Cluster has been started"))
	}

	// Check if the new NdbCluster valid is spec
	if isValid, specErrList := newNc.HasValidSpec(); !isValid {
		errList = append(errList, specErrList...)
	}

	return errList == nil, errList
}
