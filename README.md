# MySQL NDB Operator

The MySQL NDB Operator is a Kubernetes operator for managing a MySQL NDB Cluster setup inside a Kubernetes Cluster.

This is in preview state - DO NOT USE FOR PRODUCTION.

## License

Copyright (c) 2021, Oracle and/or its affiliates.

License information can be found in the LICENSE file. This distribution may include materials developed by third parties. For license and attribution notices for these materials, please refer to the LICENSE file.

## Installation

### Requirements
NDB Operator requires at least a version of 1.19.0 Kubernetes Cluster to run. Any lower version is not supported.

### Install using helm

Ndb operator comes with a helm chart that can install the NdbCluster CRD, deploy the operator and the webhook server in the K8s cluster.

Install the NDB Operator and other related resources in the `ndb-operator` namespace using :

```sh
helm install ndb-operator deploy/charts/ndb-operator \
    --namespace=ndb-operator --create-namespace
```
More information on using the helm chart is available at [deploy/charts/ndb-operator/README.md](deploy/charts/ndb-operator/README.md)

### Install using regular manifest

Create custom resource definitions, the roles and deploy the ndb operator using the single YAML file at [deploy/manifests/ndb-operator.yaml](deploy/manifests/ndb-operator.yaml).
It creates all the resources, and deploys the NDB Operator in the `ndb-operator` namespace.

```sh
kubectl apply -f deploy/manifests/ndb-operator.yaml
```

To directly use the file without cloning this entire repository, run :
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

The pod `ndb-operator-555b7b65-7fmv8` runs the NDB Operator and the other pod `ndb-operator-webhook-server-d67c97d54-zdhhp` runs a server that acts as a validator for any changes made to a NdbCluster resource.
Once both these pods are ready, we can start deploying MySQL Clusters.

## Deploy MySQL NDB Cluster in K8s Cluster

The NDB Operator relies on a Custom Resource Definition called NdbCluster to receive the configuration of a MySQL Cluster that needs to be run inside the K8s Cluster.
When a user creates, modifies or deletes a K8s object of kind NdbCluster, NDB Operator receives that change event and makes changes to the MySQL Cluster running in K8s Cluster based on that change.
The [docs/examples](docs/examples) folder in this repository has a few examples that can be used to start a MySQL Cluster in the K8s Cluster.
The documentation of the NdbCluster CRD is available at [docs/NdbCluster-CRD.md](docs/NdbCluster-CRD.md).

The [docs/examples/example-ndb.yaml](docs/examples/example-ndb.yaml) defines a simple MySQL Cluster with 2 data nodes and 2 MySQL Servers. To create this resource in the default namespace of the K8s Cluster, run :

```sh
kubectl apply -f docs/examples/example-ndb.yaml
```

Once the NdbCluster resource has been created, all pods started by the NDB Operator can be listed using :

```sh
kubectl get pods -l mysql.oracle.com/v1alpha1=example-ndb
```
The name `example-ndb` is the actual name of the resource defined in [docs/examples/example-ndb.yaml](docs/examples/example-ndb.yaml).

The output will be something similar to
```sh
NAME                                  READY   STATUS    RESTARTS   AGE
example-ndb-mgmd-0                    1/1     Running   0          3m53s
example-ndb-mgmd-1                    1/1     Running   0          2m5s
example-ndb-mysqld-599bcfbd45-b4vld   1/1     Running   0          78s
example-ndb-mysqld-599bcfbd45-bgnpz   1/1     Running   0          78s
example-ndb-ndbd-0                    1/1     Running   0          3m53s
example-ndb-ndbd-1                    1/1     Running   0          3m53s
```
Note that the MySQL pods might take some time to appear in the list as they are started only after all the Management and Data nodes are ready.

The MySQL Cluster is ready for transactions once all these pods are ready.

## Connect to the MySQL Cluster

The NDB Operator creates a few Services to give access to the MySQL Cluster nodes running inside the K8s Cluster.

```sh
kubectl get services -l mysql.oracle.com/v1alpha1=example-ndb
```

The above command will generate an output similar to :

```sh
NAME                     TYPE           CLUSTER-IP       EXTERNAL-IP   PORT(S)          AGE
example-ndb-mgmd         ClusterIP      None             <none>        1186/TCP         5m
example-ndb-mgmd-ext     LoadBalancer   10.100.164.172   <pending>     1186:30390/TCP   5m
example-ndb-mysqld-ext   LoadBalancer   10.109.197.3     <pending>     3306:32451/TCP   5m
example-ndb-ndbd         ClusterIP      None             <none>        1186/TCP         5m
```

The NDB Operator will also create a Root user with a random password on all the MySQL Servers. The root password will be stored in a K8s Secret which can be retrieved using :

```sh
kubectl get secrets -l mysql.oracle.com/v1alpha1=example-ndb
```

The expected output :
```sh
NAME                               TYPE                       DATA   AGE
example-ndb-mysqld-root-password   kubernetes.io/basic-auth   1      5m30s
```

The password can be extracted from this secret and decoded :
```sh
base64 -d <<< \
  $(kubectl get secret example-ndb-mysqld-root-password \
     -o jsonpath={.data.password})
```

A better way might be is to create the secret beforehand and set it to NdbCluster spec's `mysql.rootPasswordSecretName` field during resource creation. See the [CRD documentation](docs/NdbCluster-CRD.md#ndbmysqldspec) for more details on this.


### Access MySQL Cluster from inside K8s

To connect to the MySQL Cluster nodes from inside the K8s Cluster, you can straightaway use the `example-ndb-mgmd` or `example-ndb-mgmd-ext` service name as a connectstring to connect to the MySQL Cluster and the `example-ndb-mysqld-ext` service as the MySQL host.
The mysql, ndb_mgm clients and any other ndb tools will work as expected.

A simple demonstration is available at [docs/connect-from-inside-k8s-demo.md](docs/connect-from-inside-k8s-demo.md).

### Access MySQL Cluster from outside K8s

Both the `*-ext` Services created by the NDB Operator will be exposed via LoadBalancers if the K8s provider has the support for it and their `EXTERNAL-IP`s will be set.
The `*-mgmd-ext` and the `*-mysqld-ext` services' External-IPs can be used as NDB connectstring and MySQL host address respectively to connect to the Management and MySQL Servers.
If the LoadBalancer support is not available, one can use the `kubectl port-forward` command to access the MySQL Cluster running inside K8s.

In both the cases, unlike accessing the MySQL Cluster from inside, only the mysql and ndb_mgm clients work. Any NDB tool which uses the NDBAPI to connect to the MySQL data nodes will not work as expected.

A simple demonstration is available at [docs/connect-from-outside-k8s-demo.md](docs/connect-from-outside-k8s-demo.md).

## Updating MySQL Cluster Configuration

To update the configuration of a MySQL Cluster running in K8s, update the NdbCluster yaml spec file and then re-apply it to the K8s Cluster.
The NDB Operator will pick up the changes and apply the new configuration to the existing MySQL Cluster over sometime.
The processedGeneration field in the NdbCluster status (`.status.processedGeneration`) has the Generation number(`.metadata.Generation`) of the NdbCluster spec that was last successfully applied to the MySQL Cluster.
So a config update can be considered complete when the processedGeneration gets updated to the latest Generation number.

## Delete a MySQL Cluster

To delete the MySQL Cluster, run :
```sh
kubectl delete -f docs/examples/example-ndb.yaml
```

(or)
```sh
kubectl delete ndb example-ndb
```

## Uninstall the Operator

The NDB Operator can either be removed using helm :

```sh
helm uninstall --namespace=ndb-operator ndb-operator
kubectl delete customresourcedefinitions ndbclusters.mysql.oracle.com
```
The CRD has to be deleted separately as the helm command will not delete it.

(or)

Use the manifest file if it was installed using that :
```sh
kubectl delete -f deploy/manifests/ndb-operator.yaml
```
