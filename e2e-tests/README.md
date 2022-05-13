## Ndb Operator E2E tests

This folder contains test suites written in gingko and a tool to run the e2e tests.

### License

Copyright (c) 2021, 2022, Oracle and/or its affiliates.

License information can be found in the LICENSE file. This distribution may include materials developed by third parties. For license and attribution notices for these materials, please refer to the LICENSE file.

### The run-e2e-test.go tool

This tool can be used to run complete end to end tests.
Using a provider, this tool, creates a k8s cluster, runs the tests using ginkgo and then cleans up the cluster.
Right now two providers are implemented to work with the tool :

1. KinD provider - uses [KinD](https://kind.sigs.k8s.io/) to create a cluster to run the tests
2. local provider - uses an existing, local cluster, to run the tests

The tool is not compiled and can be run directly using 'go run'. It accepts following arguments :

```
$ go run e2e-tests/run-e2e-test.go --help
  -ginkgo.focus-file string
    	Value to be passed to the ginkgo's focus-file flag.
    	Use this to filter specs to run based on their location in files.
    	More details : https://onsi.github.io/ginkgo/#location-based-filtering
  -kind-k8s-version string
    	Kind k8s version used to run tests.
    	Example usage: -kind-k8s-version=1.20 (default "1.21")
  -kubeconfig string
    	Kubeconfig of the existing K8s cluster to run tests on.
    	This will not be used if '--use-kind' is enabled. (default "$HOME/.kube/config")
  -run-out-of-cluster
    	Enable this to run tests from outside the K8s cluster.
    	By default, this is not enabled and the tests will be run as a pod from inside K8s Cluster.
  -suites string
    	Test suites that needs to be run.
    	Example usage: -suites=mysql,basic
  -use-kind
    	Use KinD to run the e2e tests.
    	By default, this is disabled and the tests will be run in an existing K8s cluster.
  -v	Enable verbose mode in ginkgo. By default this is disabled.
```

### Running the tests

All the e2e tests are written using ginkgo and are available inside the suites folder.
These tests can be run using :
- The run-e2e-test.go tool. Eg. ```go run e2e-tests/run-e2e-test.go``` (or)
- The ginkgo tool. Eg ```go run github.com/onsi/ginkgo/ginkgo e2e-tests/suites```

### The test folders
The subdirectories inside the e2e test directory and their purpose are as follows :
- **suite** folder has the actual testcases.
- **utils** folder has various library methods used by the E2E testcases.
- **\_config** folder has all the cluster configuration used by the run-e2e-test.go to create clusters.
- **\_manifests** folder has all the yaml manifests used by tests.
- **\_artifacts** folder is used by the tests as an output directory.
