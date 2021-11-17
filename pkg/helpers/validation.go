// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package helpers

import (
	"fmt"
	"math"
	"strings"

	ndbv1alpha1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// IsValidConfig validates the Ndb resource and returns an ErrorList with all invalid field values
func IsValidConfig(newNdb *ndbv1alpha1.NdbCluster, oldNdb *ndbv1alpha1.NdbCluster) field.ErrorList {

	spec := newNdb.Spec

	var errList field.ErrorList
	specPath := field.NewPath("spec")
	mysqldPath := specPath.Child("mysqld")
	if oldNdb != nil {
		// this is an update - do not allow updating Spec.NodeCount and Spec.RedundancyLevel.
		if oldNdb.Spec.NodeCount != newNdb.Spec.NodeCount {
			var msg string
			if oldNdb.Spec.NodeCount < newNdb.Spec.NodeCount {
				msg = "Online add node is not supported by the operator yet"
			} else {
				msg = "spec.NodeCount cannot be reduced once MySQL Cluster has been started"
			}
			errList = append(errList,
				field.Invalid(specPath.Child("nodeCount"), newNdb.Spec.NodeCount, msg))
		}

		if oldNdb.Spec.RedundancyLevel != newNdb.Spec.RedundancyLevel {
			errList = append(errList,
				field.Invalid(specPath.Child("redundancyLevel"), newNdb.Spec.RedundancyLevel,
					"spec.redundancyLevel cannot be updated once MySQL Cluster has been started"))
		}

		// do not allow updating Spec.Mysqld.RootHost
		if oldNdb.Spec.Mysqld != nil &&
			newNdb.Spec.Mysqld != nil &&
			(oldNdb.Spec.Mysqld.RootHost != newNdb.Spec.Mysqld.RootHost) {
			errList = append(errList,
				field.Invalid(mysqldPath.Child("rootHost"), newNdb.Spec.Mysqld.RootHost,
					"spec.mysqld.rootHost cannot be updated once MySQL Cluster has been started"))
		}
	}

	dataNodeCount := spec.NodeCount
	mysqlServerCount := newNdb.GetMySQLServerNodeCount()
	managementNodeCount := newNdb.GetManagementNodeCount()

	// checking if number of data nodes match redundancy
	if math.Mod(float64(dataNodeCount), float64(spec.RedundancyLevel)) != 0 {
		msg := fmt.Sprintf(
			"spec.nodeCount should be a multiple of the spec.redundancyLevel(=%d)", spec.RedundancyLevel)
		errList = append(errList, field.Invalid(specPath.Child("nodeCount"), dataNodeCount, msg))
	}

	// checking total number of nodes
	total := managementNodeCount + dataNodeCount + mysqlServerCount
	if total > constants.MaxNumberOfNodes {
		invalidValue := fmt.Sprintf("%d (= %d management, %d data and %d mysql nodes)", total, managementNodeCount, dataNodeCount, mysqlServerCount)
		msg := fmt.Sprintf(
			"Total number of MySQL Cluster nodes should not exceed the allowed maximum of %d", constants.MaxNumberOfNodes)
		errList = append(errList, field.Invalid(field.NewPath("Total Nodes"), invalidValue, msg))
	}

	// validate the MySQL Root password secret name
	var rootPasswordSecret string
	if newNdb.Spec.Mysqld != nil {
		rootPasswordSecret = newNdb.Spec.Mysqld.RootPasswordSecretName
	}
	if rootPasswordSecret != "" {
		errs := validation.IsDNS1123Subdomain(rootPasswordSecret)
		// append errors, if any, to errList
		for _, err := range errs {
			errList = append(errList,
				field.Invalid(mysqldPath.Child("rootPasswordSecretName"), rootPasswordSecret, err))
		}
	}

	// validate any passed additional cnf
	myCnfString := newNdb.GetMySQLCnf()
	if len(myCnfString) > 0 {
		myCnf, err := ParseString(myCnfString)
		if err != nil && strings.Contains(err.Error(), "Non-empty line without section") {
			// section header is missing as it is optional
			// try parsing again with [mysqld] header
			myCnfString = "[mysqld]\n" + myCnfString
			myCnf, err = ParseString(myCnfString)
		}

		if err != nil {
			// error parsing the cnf
			errList = append(errList,
				field.Invalid(mysqldPath.Child("myCnf"), myCnfString, err.Error()))
		} else {
			// accept only one mysqld section in the cnf
			if len(myCnf) > 1 ||
				len(myCnf) != myCnf.GetNumberOfSections("mysqld") {
				errList = append(errList,
					field.Invalid(mysqldPath.Child("myCnf"),
						myCnfString, "spec.mysqld.myCnf can have only one mysqld section"))
			}
		}
	}

	return errList
}
