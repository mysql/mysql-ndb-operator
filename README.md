# ndb-operator

The MySQL NDB Operator is a Kubernetes operator for managing a MySQL Cluster setup inside a Kubernetes Cluster.

This is in preview state - DO NOT USE FOR PRODUCTION.

## License

Copyright (c) 2021, Oracle and/or its affiliates.

License information can be found in the LICENSE file. This distribution may include materials developed by third parties. For license and attribution notices for these materials, please refer to the LICENSE file.

## Details

The ndb controller uses [client-go library](https://github.com/kubernetes/client-go/tree/master/tools/cache) extensively.

## Fetch ndb-operator and its dependencies

The ndb-operator uses go modules and has been developed with go 1.16. 

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

## Build ndb-operator docker image

To build the ndb-operator run,

```sh
# Build ndb-operator 
make build
```

By default the operator is built in release mode. It is also possible to build the operator in debug mode by setting WITH_DEBUG environment variable to 1/ON. The debug mode can be more useful during development.

Once the operator is built, a docker image can be built by running,

```sh
# point to minikube
$ eval $(minikube docker-env)

# build ndb-operator docker image
make operator-image
```

## Build MySQL Cluster docker image (optional)

Ndb operator uses the public images available in dockerhub. By default, the mysql/mysql-cluster:latest image is used. A custom MySQL Cluster image can also be built and used with the operator. Please look at [docker/mysql-cluster/README.md](docker/mysql-cluster/README.md) for more information.

## Running Operator

**Prerequisite**: operator built, docker images built and made available in kubernetes 

### Install using helm

Ndb operator comes with a helm chart that can install the CRDs and deploy the operator and webhooks in the K8s cluster.

```sh
# Install the ndb operator and other resources in the default namespace
helm install ndb-operator deploy/charts/ndb-operator
```
More information on using the helm chart is available at [deploy/charts/ndb-operator/README.md](deploy/charts/ndb-operator/README.md)

### Install using regular manifests

Create custom resource definitions, the roles and deploy the ndb operator by applying the single YAML file - deploy/manifests/ndb-operator.yaml

```sh
# To create all the K8s resources in the default namespace
kubectl apply -f deploy/manifests/ndb-operator.yaml

# To create all the K8s resources in a custom namespace, say example-ns, run
sed -r "s/([ ]*namespace\: )default/\1example-ns/" \
  deploy/manifests/ndb-operator.yaml | kubectl apply -f -

```

Once installed, either using helm or using the yaml file, the ndb-operator and the webhook will be running in the K8s server.

## Deploy NDB Cluster in K8s Cluster

```sh
# create a custom resource of type Ndb
kubectl apply -f docs/examples/example-ndb.yaml

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

```sh
# Retrieve the Management load balancer service IP address using the service name
kubectl get service "example-ndb-mgmd-ext" \
  -o jsonpath={.status.loadBalancer.ingress[0].ip}

# (or) retrieve it using the service label
kubectl get service \
  -l "mysql.oracle.com/resource-type=mgmd-service-ext" \
  -o jsonpath={.items[0].status.loadBalancer.ingress[0].ip}

# Retrieve the MySQL Server load balancer service IP address using the service name
kubectl get service "example-ndb-mysqld-ext" \
  -o jsonpath={.status.loadBalancer.ingress[0].ip}

# (or) retrieve it using the service label
kubectl get service \
  -l "mysql.oracle.com/resource-type=mysqld-service-ext" \
  -o jsonpath={.items[0].status.loadBalancer.ingress[0].ip}

```

The MySQL Servers are also set up with a root account and a random password.
The password is stored in the k8s secret whose name will be of the format "\<ndb-cluster-name\>-mysqld-root-password".
It can be retrieved as follows :

```sh
# The password will be base64 encoded
# Retrieve it from the secret and decode it
base64 -d <<< \
  $(kubectl get secret example-ndb-mysqld-root-password \
     -o jsonpath={.data.password})
```

You can delete the cluster installation again with


```sh
kubectl delete -f docs/examples/example-ndb.yaml
```

## Cleanup

You can clean up the created CustomResourceDefinition with:

    kubectl delete crd ndbclusters.ndbcontroller.k8s.io

