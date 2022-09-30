# Building and Testing MySQL NDB Operator

The wiki describes how to build and test MySQL NDB Operator from the source code.

## License

Copyright (c) 2022, Oracle and/or its affiliates.

License information can be found in the LICENSE file. This distribution may include materials developed by third parties. For license and attribution notices for these materials, please refer to the LICENSE file.

## Prerequisites
 - [Golang v1.16](https://go.dev/dl/) or above to compile the operator.
 - [Docker](https://docs.docker.com/get-docker/) to build the NDB Operator and other container images. Docker is also used to run the E2E testcases.
 - [Minikube](https://minikube.sigs.k8s.io/docs/) or [KinD](https://kind.sigs.k8s.io/) to deploy and test the NDB Operator. A minimum version of Kubernetes 1.19 is required. If using minikube, the default memory config might not be adequate and the memory should be increased using `minikube config set` command. For example to deploy the example at [docs/examples/example-ndb.yaml](docs/examples/example-ndb.yaml), minikube requires a memory of atleast 5GB, and it can be set by `minikube config set memory 5GB`.

## Build NDB Operator

The Makefile included with the source code has all the necessary targets to build the operator.

The NDB Operator can be built using the following `make` command :
```sh
make build
```

By default, the operator is built in release mode. It can also be built in debug mode, which could be useful during development. The debug mode can be enabled by setting the `WITH_DEBUG` env variable to 1.
```ssh
WITH_DEBUG=1 make build
```

To generate the NDB Operator image on docker, run :
```ssh
make operator-image
```

Once the image is built, it has to be made accessible to the K8s Cluster. For KinD, one can use the [`kind load docker-image`](https://kind.sigs.k8s.io/docs/user/quick-start/#loading-an-image-into-your-cluster) command to directly load the image into KinD and for Minikube, it will vary depending on the container-runtime used - please read [Pushing images into minikube cluster
](https://minikube.sigs.k8s.io/docs/handbook/pushing/). Once this is done, follow the steps mentioned in the [README.md](README.md) file to deploy the NDB Operator and then to deploy a MySQL Cluster using the operator.

### Making changes to the NdbCluster type

If any change is made to the NdbCluster type at [types.go](pkg/apis/ndbcontroller/v1/types.go), the clientset, informers and the listers at [pkg/generated](pkg/generated) have to be regenerated.

To regenerate them, run :
```sh
make generate
```

The CRD and other manifest files generation is hooked up to the `make build` target and will be regenerated when you compile the code.


## Testing the operator

The NDB Operator project comes with unit tests that are developed using the go testing package and a more elaborate End-To-End(E2E) test suite that is built on top of the Ginkgo/Gomega testing framework.

### Unit Tests

The unit tests are generally co-located with the packages and test the various methods within those packages. They sometimes use fake K8s client to verify if the requests sent by the operator matches expectations.

To run all the unit tests :
```sh
make unit-test
```

### E2E Test Suite

The E2E testcases are collection of integration tests that make changes to a NdbCluster resource object and verify if the MySQL Cluster configuration controlled by the object changes accordingly. The testcases use a homegrown E2E framework built on top of the [Ginkgo](https://github.com/onsi/ginkgo) testing framework. The tests should be run using the `e2e-tests/run-e2e-test.go` tool, and they can run either in an existing K8s Cluster or start their own K8s Cluster using KinD and run the tests in them. The tool's options are documented at [e2e-tests/README.md](e2e-tests/README.md).

#### Generating the E2E tests image

To generate the e2e tests image, run :
```sh
make e2e-tests
```

#### Running the tests in an existing K8s Cluster

The e2e tests can be run in an already running K8s Cluster. This can be done by passing the kubeconfig of the Cluster to the run-e2e-test.go tool. Note that both the `mysql/ndb-operator` and the `e2e-tests` images should be accessible to the K8s Cluster for the tests to run.

```sh
go run e2e-tests/run-e2e-test.go -kubeconfig=<kubeconfig of the existing K8s Cluster>
```

#### Running the tests in their own KinD Cluster

The `run-e2e-test.go` tool can also start its own KinD Cluster and then run the e2e tests in them. Note that a docker instance with both the `mysql/ndb-operator` and the `e2e-tests` images should be accessible from the terminal where the test is being started. The KinD Cluster will be started inside docker.

```sh
go run e2e-tests/run-e2e-test.go -use-kind
```
(or)
```sh
make e2e-kind
```
