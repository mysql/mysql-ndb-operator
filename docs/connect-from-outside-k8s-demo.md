# Connect to MySQL Cluster from outside K8s

This demonstration is an extension to the example specified in the [Getting Started](getting-started.md#access-mysql-cluster-from-outside-k8s) wiki.

## Enable the MySQL Load Balancer

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

The `example-ndb-mgmd` and `example-ndb-mysqld` services are the Management and MySQL Services respectively. The management service uses an unauthenticated protocol on port 1186 and is protected by a managed NetworkPolicy. Do not expose it with a LoadBalancer. See [management-node-access.md](management-node-access.md) for controlled management access and the required CNI and namespace trust boundaries.

To enable the MySQL Server LoadBalancer, patch the NdbCluster object by running :
```sh
kubectl patch ndb example-ndb --type='merge' -p '{"spec":{"mysqlNode":{"enableLoadBalancer":true}}}'
```

Verify that the MySQL service has been upgraded :
```
NAME                 TYPE           CLUSTER-IP      EXTERNAL-IP     PORT(S)          AGE
example-ndb-mgmd     ClusterIP      10.101.29.155   <none>          1186/TCP         31m
example-ndb-mysqld   LoadBalancer   10.103.117.50   10.103.117.50   3306:32334/TCP   31m
example-ndb-ndbmtd   ClusterIP      None            <none>          1186/TCP         31m
```

The MySQL service will now be available outside the Kubernetes Cluster at the `EXTERNAL-IP` assigned by the cloud provider. Note that this demo uses minikube so the EXTERNAL-IP is same as the CLUSTER-IP, but it will vary in a production setting depending on the cloud provider used.

The MySQL Server host IP address can be extracted from the service :
```sh
mysqlHost=$(kubectl get service "example-ndb-mysqld" \
            -o jsonpath={.status.loadBalancer.ingress[0].ip})
```

Use it to start a mysql session
```sh
mysql --protocol=tcp -h $mysqlHost -u root -p
```

## Using kubectl port-forward

One can also use the `kubectl port-forward` command to access the services without enabling the LoadBalancers. Management access should be limited to trusted administrators; see [management-node-access.md](management-node-access.md) before forwarding port 1186.

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
