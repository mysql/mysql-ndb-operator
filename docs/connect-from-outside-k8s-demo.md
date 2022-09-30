# Connect to MySQL Cluster from outside K8s

This demonstration is an extension to the example specified in the [Getting Started](getting-started.md#access-mysql-cluster-from-outside-k8s) wiki.

## Enable the Load Balancers

By default, the Management and MySQL services created by the NDB Operator are of type [ClusterIP](https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types) and are accessible only from within the K8s Cluster. 

To list all the services created for the `example-ndb` NdbCluster, run :

```sh
kubectl get services -l mysql.oracle.com/v1=example-ndb
```

The output will be similar to :
```
NAME                 TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)    AGE
example-ndb-mgmd     ClusterIP   10.101.29.155   <none>        1186/TCP   4m29s
example-ndb-mysqld   ClusterIP   10.103.117.50   <none>        3306/TCP   4m29s
example-ndb-ndbmtd   ClusterIP   None            <none>        1186/TCP   4m29s
```

The `example-ndb-mgmd` and `example-ndb-mysqld` services are the Management and MySQL Services respectively. Note that they both are of type ClusterIP Service. They have to be upgraded to LoadBalancer service type to make them accessible from outside the K8s Cluster. This can be done by setting the `spec.managementNode.enableLoadBalancer` and `spec.mysqlNode.enableLoadBalancer` fields of the NdbCluster resource to true. These options can be enabled when the NdbCluster resource object is created or via an update to the object when the MySQL Cluster is already running. 

To enable both the LoadBalancers, patch the NdbCluster object by running :
```sh
kubectl patch ndb example-ndb --type='merge' -p '{"spec":{"managementNode":{"enableLoadBalancer":true},"mysqlNode":{"enableLoadBalancer":true}}}'
```

Verify that the services have been upgraded :
```
NAME                 TYPE           CLUSTER-IP      EXTERNAL-IP     PORT(S)          AGE
example-ndb-mgmd     LoadBalancer   10.101.29.155   10.101.29.155   1186:32490/TCP   31m
example-ndb-mysqld   LoadBalancer   10.103.117.50   10.103.117.50   3306:32334/TCP   31m
example-ndb-ndbmtd   ClusterIP      None            <none>          1186/TCP         31m
```

The services will now be available outside the Kubernetes Cluster at the `EXTERNAL-IP` assigned to them by the cloud provider. Not that this demo uses minikube so the EXTERNAL-IP is same as the CLUSTER-IP, but it will vary in a production setting depending on the cloud provider used.

The IP addresses can also be extracted using `kubectl`. For example to extract the Management Node service's IP, run :
```sh
ndbConnectstring=$(kubectl get service "example-ndb-mgmd" \
  -o jsonpath={.status.loadBalancer.ingress[0].ip})
```

The IP address returned by the command can be used as the connectstring to the `ndb_mgm` tool.

```sh
ndb_mgm -c $(ndbConnectstring)
```

Similarly, the MySQL Server host can be extracted from the service :
```sh
mysqlHost=$(kubectl get service "example-ndb-mysqld" \
            -o jsonpath={.status.loadBalancer.ingress[0].ip})
```

Use it to start a mysql session
```sh
mysql --protocol=tcp -h $mysqlHost -u root -p
```

## Using kubectl port-forward

One can also use the `kubectl port-forward` command to access the services without enabling the LoadBalancers.

To access the Management Server,

```sh
kubectl port-forward service/example-ndb-mgmd 1186:1186
```
This will now forward any data sent to the local 1186 port to the 1186 port of the pods running the Management Server inside the K8s Cluster. Now to connect to the Management Server, run the following command from another terminal :

```sh
ndb_mgm
```

Similarly, for MySQL Servers, forward the port by running :

```sh
kubectl port-forward service/example-ndb-mysqld 3306:3306
```

And in another terminal, connect to the MySQL Server as you would normally do with a local MySQL Server :

```sh
mysql --protocol=tcp -u root -p
```

More information on the `kubectl port-forward` command is available in the [kubectl docs](https://kubernetes.io/docs/reference/generated/kubectl/kubectl-commands#port-forward).
