package helpers

import (
	"fmt"
	"math"

	ndbv1alpha1 "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/constants"

	"k8s.io/apimachinery/pkg/util/validation"
)

// IsValidConfig checks the Ndb resources Spec for valid values
// returns NewErrorInvalidConfiguration error or nil
func IsValidConfig(ndb *ndbv1alpha1.Ndb) error {

	spec := ndb.Spec

	// checking NoOfReplicas

	if *spec.RedundancyLevel >= constants.MaxNumberOfReplicas {
		msg := fmt.Sprintf("Configured reduncany level of data is %d and exceeds the allowed maximum of %d.",
			*spec.RedundancyLevel, constants.MaxNumberOfReplicas)
		return NewErrorInvalidConfiguration(msg)
	}

	if *spec.RedundancyLevel < 1 {
		msg := fmt.Sprintf("Configured reduncany level of data is %d. Lowest level is 1 and maximum is %d.",
			*spec.RedundancyLevel, constants.MaxNumberOfReplicas)
		return NewErrorInvalidConfiguration(msg)
	}

	nc := *spec.NodeCount
	mc := spec.Mysqld.NodeCount
	msc := *spec.RedundancyLevel
	if msc > 2 {
		msc = 2
	}

	// checking number of data nodes

	if nc > constants.MaxNumberOfDataNodes {
		msg := fmt.Sprintf("Configured number of data nodes is %d and exceeds the allowed maximum of %d.",
			nc, constants.MaxNumberOfDataNodes)
		return NewErrorInvalidConfiguration(msg)
	}

	// checking if number of data nodes match reduncany
	if math.Mod(float64(nc), float64(*spec.RedundancyLevel)) != 0 {
		msg := fmt.Sprintf("Configured number of data nodes %d does not match the redundancy level %d. Number of data nodes must be a multiple of the reduncancy.",
			nc, int(*spec.RedundancyLevel))
		return NewErrorInvalidConfiguration(msg)
	}

	// checking total number of nodes

	total := nc + mc + msc
	if total > constants.MaxNumberOfNodes {
		msg := fmt.Sprintf("Configured total number of nodes is %d (= %d data, %d management and %d mysql nodes) and exceeds the allowed maximum of %d.",
			total, nc, msc, mc, constants.MaxNumberOfNodes)
		return NewErrorInvalidConfiguration(msg)
	}

	// validate the MySQL Root password secret name
	var rootPasswordSecret string
	if ndb.Spec.Mysqld != nil {
		rootPasswordSecret = ndb.Spec.Mysqld.RootPasswordSecretName
	}
	if rootPasswordSecret != "" {
		errs := validation.IsDNS1123Subdomain(rootPasswordSecret)
		if errs != nil {
			msg := fmt.Sprintf("The RootPasswordSecretName '%s' is invalid : ", rootPasswordSecret)
			for _, err := range errs {
				msg += fmt.Sprintf("%s; ", err)
			}
			return NewErrorInvalidConfiguration(msg)
		}
	}

	return nil
}
