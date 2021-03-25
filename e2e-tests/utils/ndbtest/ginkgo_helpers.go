package ndbtest

import (
	"github.com/onsi/ginkgo"
)

// DescribeFeature is simple wrapper to the ginkgo Describe.
// It additionally adds feature tag to the description
func DescribeFeature(text string, body func()) bool {
	return ginkgo.Describe("[Feature : "+text+"]", body)
}
