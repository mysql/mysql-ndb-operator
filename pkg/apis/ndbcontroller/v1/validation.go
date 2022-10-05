// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package v1

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/ndbconfig/configparser"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// List of all config parameters that are not allowed in
// .Spec.DataNode.Config. and any additional details to be appended to the error.
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
	"portnumber":        "", // Mgmd's PortNumber
	// Disallow DataDir config as that will be handled by the operator
	"datadir": "", // DataDir
}

func validateConfigParams(config map[string]*intstr.IntOrString, specPath *field.Path) (errList field.ErrorList) {
	for configKey := range config {
		if details, exists := disallowedConfigParams[strings.ToLower(configKey)]; exists {
			msg := fmt.Sprintf("config param %q is not allowed in %s. ", configKey, specPath.String())
			if details == "" {
				msg += "It will be configured automatically by the Ndb Operator based on the spec."
			} else {
				msg += details
			}
			errList = append(errList, field.Forbidden(specPath.Child(configKey), msg))
		}
	}
	return errList
}

// HasValidSpec validates the spec of the NdbCluster object
func (nc *NdbCluster) HasValidSpec() (bool, field.ErrorList) {
	spec := nc.Spec

	var errList field.ErrorList
	specPath := field.NewPath("spec")
	mysqldPath := specPath.Child("mysqlNode")
	dataNodePath := specPath.Child("dataNode")
	managementNodePath := specPath.Child("managementNode")

	dataNodeCount := spec.DataNode.NodeCount
	mysqlServerCount := nc.GetMySQLServerMaxNodeCount()
	managementNodeCount := nc.GetManagementNodeCount()
	numOfFreeApiSlots := spec.FreeAPISlots + 1

	// check if number of data nodes is a multiple of redundancy
	if math.Mod(float64(dataNodeCount), float64(spec.RedundancyLevel)) != 0 {
		msg := fmt.Sprintf(
			"spec.dataNode.nodeCount should be a multiple of the spec.redundancyLevel(=%d)", spec.RedundancyLevel)
		errList = append(errList, field.Invalid(dataNodePath.Child("nodeCount"), dataNodeCount, msg))
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

	// check if there are any disallowed config params in dataNode's Configuration.
	if err := validateConfigParams(nc.Spec.DataNode.Config, dataNodePath.Child("config")); err != nil {
		errList = append(errList, err...)
	}

	// check if there are any disallowed config params in managementNode Config.
	if nc.Spec.ManagementNode != nil {
		if err := validateConfigParams(nc.Spec.ManagementNode.Config, managementNodePath.Child("config")); err != nil {
			errList = append(errList, err...)
		}
	}

	// check if the MySQL root password secret name has the expected format
	var rootPasswordSecret string
	if spec.MysqlNode != nil {
		mysqldSpec := spec.MysqlNode

		// check if the MySQL root password secret name has the expected format
		rootPasswordSecret = mysqldSpec.RootPasswordSecretName
		if rootPasswordSecret != "" {
			errs := validation.IsDNS1123Subdomain(rootPasswordSecret)
			// append errors, if any, to errList
			for _, err := range errs {
				errList = append(errList,
					field.Invalid(mysqldPath.Child("rootPasswordSecretName"), rootPasswordSecret, err))
			}
		}

		// check if maxNodeCount is less than nodeCount
		if mysqldSpec.MaxNodeCount != 0 &&
			mysqldSpec.MaxNodeCount < mysqldSpec.NodeCount {
			msg := fmt.Sprintf(
				"spec.mysqlNode.maxNodeCount cannot be less than spec.mysqlNode.nodeCount(=%d)", mysqldSpec.NodeCount)
			errList = append(errList,
				field.Invalid(mysqldPath.Child("maxNodeCount"), mysqldSpec.MaxNodeCount, msg))
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
						myCnfString, "spec.mysqlNode.myCnf can have only one mysqld section"))
			}
		}
	}

	return errList == nil, errList
}

func cannotUpdateFieldError(specPath *field.Path, newValue interface{}) *field.Error {
	return field.Invalid(specPath, newValue,
		fmt.Sprintf("%s cannot be updated once NdbCluster has been created", specPath.String()))
}

// validateNdbPodSpecResources verifies that the Resources field of the NdbPodSpec has not changes
func validateNdbPodSpecResources(specPath *field.Path, oldNdbPodSpec, newNdbPodSpec *NdbClusterPodSpec) *field.Error {
	var oldResources, newResources *corev1.ResourceRequirements
	if oldNdbPodSpec != nil {
		oldResources = oldNdbPodSpec.Resources
	}
	if newNdbPodSpec != nil {
		newResources = newNdbPodSpec.Resources
	}

	if !reflect.DeepEqual(oldResources, newResources) {
		resourcesPath := specPath.Child("resources")
		return field.Forbidden(resourcesPath,
			fmt.Sprintf("%s cannot be updated once NdbCluster has been created", resourcesPath.String()))
	}

	return nil
}

func (nc *NdbCluster) IsValidSpecUpdate(newNc *NdbCluster) (bool, field.ErrorList) {

	var errList field.ErrorList
	specPath := field.NewPath("spec")
	managementNodePath := specPath.Child("managementNode")
	dataNodePath := specPath.Child("dataNode")
	mysqldPath := specPath.Child("mysqlNode")

	if nc.Spec.RedundancyLevel == 1 {
		// MySQL Cluster replica = 1 => updating MySQL config via
		// rolling restart is not possible. Disallow any spec update.
		errList = append(errList,
			field.InternalError(specPath,
				errors.New("operator cannot handle any spec update to a MySQL Cluster whose replica is 1")))
		return false, errList
	}

	// Do not allow decreasing Spec.DataNode.NodeCount
	if nc.Spec.DataNode.NodeCount > newNc.Spec.DataNode.NodeCount {
		errList = append(errList,
			field.Invalid(dataNodePath.Child("nodeCount"), newNc.Spec.DataNode.NodeCount,
				"spec.dataNode.nodeCount cannot be reduced once MySQL Cluster has been started"))
	}

	// Do not allow updating Spec.RedundancyLevel
	if nc.Spec.RedundancyLevel != newNc.Spec.RedundancyLevel {
		errList = append(errList,
			cannotUpdateFieldError(specPath.Child("redundancyLevel"), newNc.Spec.RedundancyLevel))
	}

	// Do not allow updating Resource field of various ndbPodSpecs
	if nc.Spec.ManagementNode != nil {
		if err := validateNdbPodSpecResources(
			managementNodePath.Child("ndbPodSpec"),
			nc.Spec.ManagementNode.NdbPodSpec, newNc.Spec.ManagementNode.NdbPodSpec); err != nil {
			errList = append(errList, err)
		}
	}
	if err := validateNdbPodSpecResources(
		dataNodePath.Child("ndbPodSpec"), nc.Spec.DataNode.NdbPodSpec, newNc.Spec.DataNode.NdbPodSpec); err != nil {
		errList = append(errList, err)
	}
	if nc.GetMySQLServerNodeCount() != 0 &&
		newNc.GetMySQLServerNodeCount() != 0 {
		if err := validateNdbPodSpecResources(
			mysqldPath.Child("ndbPodSpec"), nc.Spec.MysqlNode.NdbPodSpec, newNc.Spec.MysqlNode.NdbPodSpec); err != nil {
			errList = append(errList, err)
		}
	}

	if nc.GetMySQLServerConnectionPoolSize() > newNc.GetMySQLServerConnectionPoolSize() {
		// Do not allow reducing connection pool size as that leads to chaos when reserving nodeIds
		errList = append(errList,
			field.Invalid(mysqldPath.Child("connectionPoolSize"),
				newNc.GetMySQLServerConnectionPoolSize(),
				"connectionPoolSize cannot be reduced once MySQL Cluster has been started"))
	}

	// Check if the new NdbCluster valid is spec
	if isValid, specErrList := newNc.HasValidSpec(); !isValid {
		errList = append(errList, specErrList...)
	}

	return errList == nil, errList
}
