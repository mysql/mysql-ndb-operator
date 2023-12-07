# MySQL NDB Operator

The MySQL NDB Operator is a Kubernetes operator for managing a MySQL NDB Cluster setup inside a Kubernetes Cluster.

## License

Copyright (c) 2021, 2024, Oracle and/or its affiliates.

License information can be found in the LICENSE file. This distribution may include materials developed by third parties. For license and attribution notices for these materials, please refer to the LICENSE file.

## Installation

### Prerequisites
 - Kubernetes Version 1.19+

### Install using helm

Ndb operator comes with a [Helm](https://helm.sh/docs/intro/quickstart/) chart that can install the NdbCluster CRD, deploy the operator and the webhook server in the K8s cluster.

Add the NDB Operator helm chart repository :

```sh
helm repo add ndb-operator-repo https://mysql.github.io/mysql-ndb-operator/
helm repo update
```

Install the NDB Operator and other related resources in the `ndb-operator` namespace using :

```sh
helm install ndb-operator ndb-operator-repo/ndb-operator \
    --namespace=ndb-operator --create-namespace
```
More information on using the helm chart is available at [deploy/charts/ndb-operator/README.md](deploy/charts/ndb-operator/README.md)

### Install using kubectl

Create custom resource definitions, the roles and deploy the ndb operator using the single YAML file at [deploy/manifests/ndb-operator.yaml](deploy/manifests/ndb-operator.yaml).
It creates all the resources, and deploys the NDB Operator in the `ndb-operator` namespace.

```sh
kubectl apply -f deploy/manifests/ndb-operator.yaml
```

To directly apply the manifest file without cloning this entire repository, run :
```sh
kubectl apply -f https://raw.githubusercontent.com/mysql/mysql-ndb-operator/main/deploy/manifests/ndb-operator.yaml
```

To run the operator in a different namespace, the manifest file has to be updated before applying it to the K8s Server.

### Verify Installation

Once installed, either using helm or using the yaml file, the ndb-operator and a webhook server will be running in the K8s server.
To verify it, run the following in the namespace they were installed :

```sh
kubectl get pods -n ndb-operator -l 'app in (ndb-operator,ndb-operator-webhook-server)'
```
Output will be similar to :

```sh
NAME                                          READY   STATUS    RESTARTS   AGE
ndb-operator-555b7b65-7fmv8                   1/1     Running   0          13s
ndb-operator-webhook-server-d67c97d54-zdhhp   1/1     Running   0          13s
```

The pod `ndb-operator-555b7b65-7fmv8` runs the NDB Operator and the other pod `ndb-operator-webhook-server-d67c97d54-zdhhp` runs a server that acts as an admission controller for the NdbCluster resource. The NDB Operator is ready to handle NdbCluster resource when both these pods are ready.

## Deploy the example MySQL NDB Cluster

The configuration of the MySQL Cluster to be deployed in the K8s Cluster can be defined using the NdbCluster Custom resource. The example at [docs/examples/example-ndb.yaml](docs/examples/example-ndb.yaml) defines a simple MySQL Cluster with 2 data nodes and 2 MySQL Servers. To create this object in the default namespace of the K8s Cluster, run :

```sh
kubectl apply -f docs/examples/example-ndb.yaml
```
The NDB Operator will now deploy a MySQL Cluster based on the configuration defined in the NdbCluster resource. Checkout the [Getting Started](docs/getting-started.md) wiki for more documentation on configuring and accessing the data from the MySQL Cluster.

## Uninstall the Operator

The NDB Operator can either be removed using helm :

```sh
helm uninstall --namespace=ndb-operator ndb-operator
kubectl delete customresourcedefinitions ndbclusters.mysql.oracle.com
```
Note : The CRD has to be deleted separately as the helm command will not delete it.

(or)

Use the manifest file if the operator was installed using that :
```sh
kubectl delete -f deploy/manifests/ndb-operator.yaml
```

## Contributing

The MySQL team welcomes ideas, contribution and feedback from the community. Please read the [CONTRIBUTING.md](CONTRIBUTING.md) and [DEVELOPER.md](DEVELOPER.md) files for more information on this topic.
