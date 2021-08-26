# Connect to MySQL Cluster from outside K8s

This demonstration is an extension to the example specified in the [README.md](../README.md#deploy-mysql-ndb-cluster-in-k8s-cluster) file.

## Using LoadBalancer Services

The operator creates LoadBalancer services to allow access to the Management server and the MySQL Servers running inside the K8s cluster.

The load balancer service names will be of the following format :
 * Management Server LoadBalancer : "\<NdbCluster.Name\>-mgmd-ext"
 * MySQL Server LoadBalancer : "\<NdbCluster.Name\>-mysqld-ext"

If the K8s provider has support for LoadBalancer provisioning, the services will be assigned a LoadBalancer with an external IP.

```sh
kubectl get services -l mysql.oracle.com/v1alpha1=example-ndb

NAME                     TYPE           CLUSTER-IP       EXTERNAL-IP      PORT(S)          AGE
example-ndb-mgmd         ClusterIP      None             <none>           1186/TCP         10m2s
example-ndb-mgmd-ext     LoadBalancer   10.100.164.172   10.100.164.172   1186:30390/TCP   10m2s
example-ndb-mysqld-ext   LoadBalancer   10.109.197.3     10.109.197.3     3306:32451/TCP   10m2s
example-ndb-ndbd         ClusterIP      None             <none>           1186/TCP         10m2s
```

The IP addresses can also be extracted using `kubectl`
```sh
ndbConnectstring=$(kubectl get service "example-ndb-mgmd-ext" \
  -o jsonpath={.status.loadBalancer.ingress[0].ip})
```

The IP address returned by the command can be used as the connectstring to the `ndb_mgm` tool.

```sh
ndb_mgm -c $(ndbConnectstring)
```

Similarly, the MySQL Server LoadBalancer service can be used to access the MySQL Server.
```sh
mysqlHost=$(kubectl get service "example-ndb-mysqld-ext" \
            -o jsonpath={.status.loadBalancer.ingress[0].ip})
```

Use it to start a mysql session
```sh
mysql --protocol=tcp -h $mysqlHost -u root -p
```

## Using kubectl port-forward

Another way is to use the `kubectl port-forward` command to forward a port from the local machine to a port inside the K8s node.

For the Management Server,

```sh
kubectl port-forward service/example-ndb-mgmd 1186:1186
```

Then in another terminal connect to the Management Server using
```sh
ndb_mgm
```

Similarly, for MySQL Servers, forward the port by issuing :

```sh
kubectl port-forward service/example-ndb-mysqld-ext 3306:3306
```

And in another terminal, connect to the MySQL Server :

```sh
mysql --protocol=tcp -u root -p
```

More information on the `kubectl port-forward` command is available in the [kubectl docs](https://kubernetes.io/docs/reference/generated/kubectl/kubectl-commands#port-forward).
