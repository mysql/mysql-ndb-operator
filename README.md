# ndb-operator

This repository implements a simple controller for watching Ndb resources as
defined with a CustomResourceDefinition (CRD). 

This is pre-alpha - mostly prototyping for testing concepts.

## Details

The ndb controller uses [client-go library](https://github.com/kubernetes/client-go/tree/master/tools/cache) extensively.

## Fetch ndb-operator and its dependencies

The ndb-operator uses go modules and has been developed with go 1.13. 

```sh
git clone <this repo>
cd ndb-operator
```

## Changes to the types

If you intend to change the types then you will need to generate code and manifests again.

The project uses two generators :
- [k8s.io/code-generator](https://github.com/kubernetes/code-generator) to generate a typed client, informers, listers and deep-copy functions.
- [controller-gen](https://github.com/kubernetes-sigs/controller-tools/tree/master/cmd/controller-gen) to generate the CRDs.

To generate the typed client, informers, listers and deep-copy functions run,
```sh
make generate
```

To update the CRD definitions based on the changes made to the types run,
```sh
make manifests
```

## Building

To build the ndb-operator run,

```sh
# Build ndb-operator 
make build
```

## Docker image building

You can build your own ndb cluster images but you don't have to. Currently public image 8.0.22 is used.

**Prerequisite**: You have a build directory available that is build for OL8.
You can use a OL8 build-container in docker/ol8 for that or download a readily compiled OL8 build.

If you use minikube then set the environment to minikube first before building the image.

```sh
# point to minikube
$ eval $(minikube docker-env)

# build docker image
# BASEDIR is the MySQL Cluster build directory
# IMAGE_TAG is the string to be tagged to the container image being built
BASEDIR=<basedir> IMAGE_TAG=<build-tag> make ndb-container-image
```

## Running

**Prerequisite**: operator built, docker images built and made available in kubernetes 

Create custom resource definitions and the roles:

```sh
NAMESPACE=default
kubectl -n ${NAMESPACE} apply -f artifacts/manifests/crd.yaml
sed -e "s/<NAMESPACE>/${NAMESPACE}/g" artifacts/manifests/rbac.yaml | kubectl -n ${NAMESPACE} apply -f -
```

or use helm

```sh
helm install ndb-operator helm
```

Then proceed with starting the operator - currently outside of the k8 cluster:

```sh
make run
(or)
./bin/<path-to-operator>/ndb-operator -kubeconfig=$HOME/.kube/config

# create a custom resource of type Ndb
kubectl apply -f artifacts/examples/example-ndb.yaml

# check statefulsets created through the custom resource
kubectl get pods,statefulsets

# watch pods change state
kubectl get pods -w

# "log into" pods with 
kubectl exec -ti pod/example-ndb-mgmd-0 -- /bin/bash
```

The operator creates loadbalancer services to allow access to the Management server and the MySQL Servers running inside the K8s cluster.
The load balancer service names will be of the following format :
 * Management Server load balancer : "\<ndb-cluster-name\>-mgmd-ext"
 * MySQL Server loadbalancer : "\<ndb-cluster-name\>-mysqld-ext"

```
# Retrieve the Management load balancer service IP address using the service name
kubectl get service "example-ndb-mgmd-ext" \
  -o jsonpath={.status.loadBalancer.ingress[0].ip}

# (or) retrieve it using the service label
kubectl get service \
  -l "mysql.oracle.com/resourcetype=mgmd-service-ext" \
  -o jsonpath={.items[0].status.loadBalancer.ingress[0].ip}

# Retrieve the MySQL Server load balancer service IP address using the service name
kubectl get service "example-ndb-mysqld-ext" \
  -o jsonpath={.status.loadBalancer.ingress[0].ip}

# (or) retrieve it using the service label
kubectl get service \
  -l "mysql.oracle.com/resourcetype=mysqld-service-ext" \
  -o jsonpath={.items[0].status.loadBalancer.ingress[0].ip}

```

The MySQL Servers are also set up with a root account and a random password.
The password is stored in the k8s secret whose name will be of the format "\<ndb-cluster-name\>-mysqld-root-password".
It can be retrieved as follows :

```
# The password will be base64 encoded
# Retrieve it from the secret and decode it
base64 -d <<< \
  $(kubectl get secret example-ndb-mysqld-root-password \
     -o jsonpath={.data.password})
```

You can delete the cluster installation again with


```sh
kubectl delete -f artifacts/examples/example-ndb.yaml
```


## Defining types

The Ndb instance of our custom resource has an attached Spec, 
which is defined in [types.go](pkg/apis/ndbcontroller/types.go)

## Validation

To validate custom resources, use the [`CustomResourceValidation`](https://kubernetes.io/docs/tasks/access-kubernetes-api/extend-api-custom-resource-definitions/#validation) feature.

This feature is beta and enabled by default in v1.9.

### Example

The schema in [`crd-validation.yaml`](./artifacts/examples/crd-validation.yaml) applies the following validation on the custom resource:
`spec.replicas` must be an integer and must have a minimum value of 1 and a maximum value of 10.

In the above steps, use `crd-validation.yaml` to create the CRD:

```sh
# create a CustomResourceDefinition supporting validation
kubectl create -f artifacts/examples/crd-validation.yaml
```

## Subresources

TBD

## Cleanup

You can clean up the created CustomResourceDefinition with:

    kubectl delete crd ndbs.ndbcontroller.k8s.io



