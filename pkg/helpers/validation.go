package helpers

import (
	"fmt"
	"math"
	"strings"

	ndbv1alpha1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"
	"github.com/mysql/ndb-operator/pkg/helpers/ndberrors"

	"k8s.io/apimachinery/pkg/util/validation"
)

// IsValidConfig validates the Ndb resource and returns an error with all invalid field values
func IsValidConfig(ndb *ndbv1alpha1.Ndb) error {

	spec := ndb.Spec

	dataNodeCount := spec.NodeCount
	mysqlServerCount := ndb.GetMySQLServerNodeCount()
	managementNodeCount := spec.RedundancyLevel
	if managementNodeCount > 2 {
		managementNodeCount = 2
	}

	errBuilder := ndberrors.NewInvalidConfigNdbErrorBuilder()

	// checking if number of data nodes match redundancy
	if math.Mod(float64(dataNodeCount), float64(spec.RedundancyLevel)) != 0 {
		msg := fmt.Sprintf("spec.nodeCount should be a multiple of the spec.redundancyLevel(=%d)", spec.RedundancyLevel)
		errBuilder.AddInvalidField("spec.nodeCount", fmt.Sprint(dataNodeCount), msg)
	}

	// checking total number of nodes
	total := managementNodeCount + dataNodeCount + mysqlServerCount
	if total > constants.MaxNumberOfNodes {
		invalidValue := fmt.Sprintf("%d (= %d management, %d data and %d mysql nodes)", total, managementNodeCount, dataNodeCount, mysqlServerCount)
		msg := fmt.Sprintf("Total nodes should not exceed the allowed maximum of %d", constants.MaxNumberOfNodes)
		errBuilder.AddInvalidField("Total Nodes", invalidValue, msg)
	}

	// validate the MySQL Root password secret name
	var rootPasswordSecret string
	if ndb.Spec.Mysqld != nil {
		rootPasswordSecret = ndb.Spec.Mysqld.RootPasswordSecretName
	}
	if rootPasswordSecret != "" {
		errs := validation.IsDNS1123Subdomain(rootPasswordSecret)
		if errs != nil {
			for _, err := range errs {
				errBuilder.AddInvalidField("spec.mysqld.rootPasswordSecretName", rootPasswordSecret, err)
			}
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
			errBuilder.AddInvalidField("spec.mysqld.additionalCnf", myCnfString, err.Error())
		} else {
			// accept only one mysqld section in the cnf
			if len(myCnf) > 1 ||
				len(myCnf) != myCnf.GetNumberOfSections("mysqld") {
				msg := "spec.mysqld.additionalCnf can have only one mysqld section"
				errBuilder.AddInvalidField("spec.mysqld.additionalCnf", myCnfString, msg)
			}
		}
	}

	return errBuilder.NdbError()
}
