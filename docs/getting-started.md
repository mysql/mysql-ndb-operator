# Getting Started with the NDB Operator

## NdbCluster Custom Resource

The NdbCluster custom resource is defined with custom fields that map to the configuration of a MySQL Cluster. To deploy a MySQL Cluster inside the K8s Cluster, an NdbCluster object with the desired configuration of the MySQL Cluster has to be created in the K8s Cluster. The NDB Operator watches for any changes made to any NdbCluster objects and will receive an event when the NdbCluster object is created in the K8s Cluster. It will then start deploying the MySQL Cluster, with the specified configuration, inside the K8s Cluster using native Kubernetes workloads and resources. Once the MySQL Cluster is ready, the NdbCluster resource can be further modified to make changes to the MySQL Cluster configuration. The Operator will receive this event as well and will update the configuration of the already running MySQL Cluster.

The documentation at [NdbCluster-CRD.md](NdbCluster-CRD.md) explains the various custom fields of the NdbCluster CRD and the effect they have on the MySQL Cluster configuration. The [examples](examples) folder has a few examples of the NdbCluster resource objects which can be used to deploy MySQL Cluster inside the K8s Cluster.

## Deploy a simple MySQL Cluster

The [examples/example-ndb.yaml](examples/example-ndb.yaml) file has a NdbCluster object `example-ndb` that defines a simple MySQL Cluster with 2 data nodes and 2 MySQL Servers. To create this object in the default namespace of the K8s Cluster, run this command from the root of the repo :

```sh
kubectl apply -f docs/examples/example-ndb.yaml
```

Various NdbCluster status fields report back the actual status of the MySQL Cluster running inside the K8s Cluster. Wait for the MySQL Cluster nodes to be started and become ready using the [UpToDate Condition](NdbCluster-CRD.md#ndbclusterconditiontypestring-alias) defined in the NdbCluster object status :
```sh
kubectl wait --for=condition=UpToDate ndb example-ndb --timeout=10m
```

The MySQL Cluster is ready for transactions once the NdbCluster is `UpToDate`, i.e. the `UpToDate` condition becomes true.

At any point, a brief status of an NdbCluster object can be viewed using `kubectl get ndb` command.
```sh
kubectl get ndb example-ndb
```
The output of the command gives a short summary the MySQL Cluster nodes controlled by the respective NdbCluster object.
```
NAME          REPLICA   MANAGEMENT NODES   DATA NODES   MYSQL SERVERS   AGE   UP-TO-DATE
example-ndb   2         Ready:2/2          Ready:2/2    Ready:2/2       3m    True
```
To list all the pods created by the NDB Operator, run :

```sh
kubectl get pods -l mysql.oracle.com/v1alpha1=example-ndb
```

For `example-ndb` NdbCluster object, two management nodes (\*-mgmd-\* pods), two multi-threaded data nodes (\*-ndbd-\* pods) and two MySQL Servers (\*-mysqld-\*) would have been created by the NDB operator :
```sh
NAME                                  READY   STATUS    RESTARTS   AGE
example-ndb-mgmd-0                    1/1     Running   0          3m53s
example-ndb-mgmd-1                    1/1     Running   0          2m5s
example-ndb-mysqld-599bcfbd45-b4vld   1/1     Running   0          78s
example-ndb-mysqld-599bcfbd45-bgnpz   1/1     Running   0          78s
example-ndb-ndbd-0                    1/1     Running   0          3m53s
example-ndb-ndbd-1                    1/1     Running   0          3m53s
```

## Connect to the MySQL Cluster

The NDB Operator, by default, creates few Services to expose the services offered by the MySQL Cluster nodes within the Kubernetes Cluster.

```sh
kubectl get services -l mysql.oracle.com/v1alpha1=example-ndb
```

The above command will generate an output similar to :

```sh
NAME                 TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
example-ndb-mgmd     ClusterIP   10.100.207.255   <none>        1186/TCP   5m
example-ndb-mysqld   ClusterIP   10.103.182.157   <none>        3306/TCP   5m
example-ndb-ndbd     ClusterIP   None             <none>        1186/TCP   5m
```

The NDB Operator will also create a `root` user with a random password on all the MySQL Servers. The root password will be stored in a K8s Secret which can be retrieved from the NdbCluster object status :

```sh
kubectl get ndb example-ndb -o jsonpath={.status.generatedRootPasswordSecretName}
```

To extract and decode the password from the Secret :
```sh
base64 -d <<< \
  $(kubectl get secret example-ndb-mysqld-root-password \
     -o jsonpath={.data.password})
```
One can also specify the password and host to be used with the `root` account via the NdbCluster spec. See the [CRD documentation](NdbCluster-CRD.md#ndbmysqldspec) for more details on this.

### Access MySQL Cluster from inside K8s

To connect to the Management and MySQL Servers from within the Kubernetes Cluster,  you can straightaway use the `example-ndb-mgmd` and `example-ndb-mysqld` services as the MySQL Cluster connectstring and MySQL host respectively.
The mysql, ndb_mgm clients and any other ndb tools will work as expected.

A demonstration is available at [connect-from-inside-k8s-demo.md](connect-from-inside-k8s-demo.md).

### Access MySQL Cluster from outside K8s

By default, the Management and MySQL services created by the NDB Operator are of type [ClusterIP](https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types) and are accessible only from within the K8s Cluster. To expose them outside the K8s Cluster, the services have to be upgraded into a `LoadBalancer` type. This can be done by setting the `spec.enableManagementNodeLoadBalancer` and `spec.mysqld.enableLoadBalancer` fields of the NdbCluster resource to true. These options can be enabled when the NdbCluster resource object is created or via an update to the object when the MySQL Cluster is already running.

Once enabled, the previously ClusterIP type `example-ndb-mgmd` and `example-ndb-mysqld` services will be upgraded to LoadBalancer type, and they will be available at the external IP assigned to them by the cloud provider.

Another way to access these services without enabling the LoadBalancers support is to use the `kubectl port-forward` command.

In both ways, only the mysql and ndb_mgm clients work. Any NDB tool which uses the NDBAPI to connect to the MySQL data nodes will not work as expected from outside the K8s Cluster.

A demonstration is available at [connect-from-outside-k8s-demo.md](connect-from-outside-k8s-demo.md).

## Updating MySQL Cluster Configuration

Once a MySQL Cluster has been deployed by the NDB Operator inside the K8s Cluster, its configuration can be updated by updating the spec of the NdbCluster resource object that represents it. The NDB Operator will pick up any changes made to the NdbCluster resource object and will apply the updated configuration to the respective MySQL Cluster. The NDB Operator takes care of updating the MySQL Cluster config file and restarting all the management/data nodes if required. When the update is being handled by the NdbOperator, it will set the `UpToDate` condition of the NdbCluster resource object to false and will not accept any further updates until it completes the current one.

The `kubectl get ndb example-ndb` command will report the `UpToDate` status of an NdbCluster. It can be used to check if the current update has been completed.
```
$ kubectl get ndb example-ndb
NAME          REPLICA   MANAGEMENT NODES   DATA NODES   MYSQL SERVERS   AGE   UP-TO-DATE
example-ndb   2         Ready:2/2          Ready:1/2    Ready:2/2       10m   False
```

The `UpToDate` condition can also be used to wait for the operator to complete the update.
```sh
kubectl wait --for=condition=UpToDate ndb example-ndb --timeout=10m
```

Once the update is complete, the `UpToDate` condition will be set back to true by the operator.
```
$ kubectl get ndb example-ndb
NAME          REPLICA   MANAGEMENT NODES   DATA NODES   MYSQL SERVERS   AGE      UP-TO-DATE
example-ndb   2         Ready:2/2          Ready:2/2    Ready:2/2       10m50s   True
```

## Delete a MySQL Cluster
To stop and remove the MySQL Cluster running inside the K8s Cluster, delete the NdbCluster resource object.

```sh
kubectl delete -f docs/examples/example-ndb.yaml
```
(or)
```sh
kubectl delete ndb example-ndb
```
This will delete all the data, the pods running the MySQL Cluster nodes and also delete all other associated K8s resources created by the NDB Operator.
