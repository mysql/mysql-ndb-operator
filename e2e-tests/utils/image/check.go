package image

import (
	"context"
	"os"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"k8s.io/klog"
)

// CheckOperatorImage tries to find the ndb-operator image
// simply exits the program if not found
func CheckOperatorImage() {
	cli, err := client.NewEnvClient()
	if err != nil {
		klog.Fatalf("Unable to connect to docker instance: %s\n", err)
		os.Exit(1)
	}

	images, err := cli.ImageList(context.Background(), types.ImageListOptions{})
	if err != nil {
		klog.Fatalf("Unable to retrieve image list from docker registry: %s\n", err)
		os.Exit(1)
	}

	findOperator := false
	for _, image := range images {
		for _, repoTag := range image.RepoTags {
			if strings.HasPrefix(repoTag, "ndb-operator") {
				findOperator = true
			}
		}
	}

	if !findOperator {
		klog.Fatalf("No operator image ndb-operator available\n")
		os.Exit(1)
	}
}
