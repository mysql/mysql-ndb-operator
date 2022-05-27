// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package v1alpha1

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/ndbconfig/configparser"

	corev1 "k8s.io/api/core/v1"
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

func cannotUpdateFieldError(specPath *field.Path, newValue interface{}) *field.Error {
	return field.Invalid(specPath, newValue,
		fmt.Sprintf("%s cannot be updated once NdbCluster has been created", specPath.String()))
}

// validateNdbPodSpecResources verifies that the Resources field of the NdbPodSpec has not changes
func validateNdbPodSpecResources(specPath *field.Path, oldNdbPodSpec, newNdbPodSpec *NdbPodSpec) *field.Error {
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
			cannotUpdateFieldError(specPath.Child("redundancyLevel"), newNc.Spec.RedundancyLevel))
	}

	// Do not allow updating Resource field of various ndbPodSpecs
	if err := validateNdbPodSpecResources(
		specPath.Child("managementNodePodSpec"),
		nc.Spec.ManagementNodePodSpec, newNc.Spec.ManagementNodePodSpec); err != nil {
		errList = append(errList, err)
	}
	if err := validateNdbPodSpecResources(
		specPath.Child("dataNodePodSpec"), nc.Spec.DataNodePodSpec, newNc.Spec.DataNodePodSpec); err != nil {
		errList = append(errList, err)
	}
	if nc.GetMySQLServerNodeCount() != 0 &&
		newNc.GetMySQLServerNodeCount() != 0 {
		if err := validateNdbPodSpecResources(
			mysqldPath.Child("podSpec"), nc.Spec.Mysqld.PodSpec, newNc.Spec.Mysqld.PodSpec); err != nil {
			errList = append(errList, err)
		}
	}

	// Check if the new NdbCluster valid is spec
	if isValid, specErrList := newNc.HasValidSpec(); !isValid {
		errList = append(errList, specErrList...)
	}

	return errList == nil, errList
}
