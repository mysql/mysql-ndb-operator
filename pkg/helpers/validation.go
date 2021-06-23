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
func IsValidConfig(ndb *ndbv1alpha1.Ndb) field.ErrorList {

	spec := ndb.Spec

	dataNodeCount := spec.NodeCount
	mysqlServerCount := ndb.GetMySQLServerNodeCount()
	managementNodeCount := ndb.GetManagementNodeCount()

	var errList field.ErrorList
	specPath := field.NewPath("spec")

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
	mysqldPath := specPath.Child("mysqld")
	var rootPasswordSecret string
	if ndb.Spec.Mysqld != nil {
		rootPasswordSecret = ndb.Spec.Mysqld.RootPasswordSecretName
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
	myCnfString := ndb.GetMySQLCnf()
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
