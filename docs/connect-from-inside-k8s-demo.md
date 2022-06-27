# Connect to MySQL Cluster from inside K8s

This demonstration is an extension to the example specified in the [Getting Started](getting-started.md#access-mysql-cluster-from-inside-k8s) wiki.

Exec into a pod that has the required client tools. Note : A MySQL Server pod started by the NDB Operator is used here. But the methods described here should work on any pod running inside the K8s Server that has the required MySQL and NDB clients.

```sh
kubectl exec -it example-ndb-mysqld-0 -- bash
```

Connect to the Management Server using the `example-ndb-mgmd` service created by the NDB Operator, as a connectstring :

```sh
ndb_mgm -c example-ndb-mgmd
```

or, if the `ndb_mgm` client is running in a pod from a different namespace than that of the Service, use the Service's DNS name as explained in [https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#services](https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/#services) as the connectstring.

```sh
ndb_mgm -c example-ndb-mgmd.default.svc.cluster.local
```

Once it is running, it can be used to extract information about the MySQL Cluster nodes.

```sh
$ ndb_mgm -c example-ndb-mgmd
-- NDB Cluster -- Management Client --
ndb_mgm> show
Connected to Management Server at: example-ndb-mgmd:1186
Cluster Configuration
---------------------
[ndbd(NDB)] 2 node(s)
id=3    @172.17.0.6  (mysql-8.0.26 ndb-8.0.26, Nodegroup: 0, *)
id=4    @172.17.0.7  (mysql-8.0.26 ndb-8.0.26, Nodegroup: 0)

[ndb_mgmd(MGM)] 2 node(s)
id=1    @172.17.0.5  (mysql-8.0.26 ndb-8.0.26)
id=2    @172.17.0.8  (mysql-8.0.26 ndb-8.0.26)

[mysqld(API)]   5 node(s)
id=145  @172.17.0.10  (mysql-8.0.26 ndb-8.0.26)
id=146  @172.17.0.9  (mysql-8.0.26 ndb-8.0.26)
id=147 (not connected, accepting connect from any host)
id=148 (not connected, accepting connect from any host)
id=149 (not connected, accepting connect from any host)

ndb_mgm> exit
```

Similarly connect to the MySQL Server using the `example-ndb-mysqld` service using the root account.
If the root account password was not passed to the NdbCluster object, then it should be extracted as explained in the [Getting Started](getting-started.md#connect-to-the-mysql-cluster) wiki.

```sh
mysql --protocol=tcp -h example-ndb-mysqld -u root -p
```
(or)
```sh
mysql --protocol=tcp -h example-ndb-mysqld.default.svc.cluster.local -u root -p
```
if the pod is from a different namespace.

Once connected, the client can run the queries as usual :
```
Welcome to the MySQL monitor.  Commands end with ; or \g.
Your MySQL connection id is 500
Server version: 8.0.26-cluster MySQL Cluster Community Server - GPL

Copyright (c) 2000, 2021, Oracle and/or its affiliates.

Oracle is a registered trademark of Oracle Corporation and/or its
affiliates. Other names may be trademarks of their respective
owners.

Type 'help;' or '\h' for help. Type '\c' to clear the current input statement.

mysql> select * from information_schema.engines where engine like "ndb%";
+------------+---------+-------------------------------------------------+--------------+------+------------+
| ENGINE     | SUPPORT | COMMENT                                         | TRANSACTIONS | XA   | SAVEPOINTS |
+------------+---------+-------------------------------------------------+--------------+------+------------+
| ndbcluster | YES     | Clustered, fault-tolerant tables                | YES          | NO   | NO         |
| ndbinfo    | YES     | MySQL Cluster system information storage engine | NO           | NO   | NO         |
+------------+---------+-------------------------------------------------+--------------+------+------------+
2 rows in set (0.00 sec)

mysql> create database test;
Query OK, 1 row affected (0.10 sec)

mysql> use test;
Database changed

mysql> create table cities (
    ->   id int primary key auto_increment,
    ->   name char(50),
    ->   population int
    -> ) engine ndb;
Query OK, 0 rows affected (0.33 sec)

mysql> insert into cities (name, population) values
    ->   ('Bengaluru', 8425970),
    ->   ('Chennai', 4646732),
    ->   ('Mysuru', 920550),
    ->   ('Tirunelveli', 968984);
Query OK, 4 rows affected (0.02 sec)
Records: 4  Duplicates: 0  Warnings: 0

mysql> select * from cities order by id;
+----+-------------+------------+
| id | name        | population |
+----+-------------+------------+
|  1 | Bengaluru   |    8425970 |
|  2 | Chennai     |    4646732 |
|  3 | Mysuru      |     920550 |
|  4 | Tirunelveli |     968984 |
+----+-------------+------------+
4 rows in set (0.02 sec)
```

Similarly, any NDB tool that uses the NDBAPI can use the `example-ndb-mgmd` service to connect to the MySQL Cluster. For example, to use `ndb_select_all` tool to query the cities table, run :

```sh
ndb_select_all -c example-ndb-mgmd -d test cities --order PRIMARY
```

Output of the command :

```
id	name	        population
1	"Bengaluru"     8425970
2	"Chennai"       4646732
3	"Mysuru"        920550
4	"Tirunelveli"   968984
4 rows returned

NDBT_ProgramExit: 0 - OK
```
