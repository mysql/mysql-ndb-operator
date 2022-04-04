# Wordpress on NDB

Contains resource files to run a wordpress app in K8s utilizing ndb resources
created using ndb-operator.

## License

Copyright (c) 2021, 2022 Oracle and/or its affiliates.

License information can be found in the LICENSE file. This distribution may include materials developed by third parties. For license and attribution notices for these materials, please refer to the LICENSE file.

## Resource files details

### **wordpress-ndb.yaml**

Deploys a MySQL Cluster with two Management Nodes, two Data Nodes and two MySQL Servers.
Services to access ndb cluster is created. Generates a "secret" that will be used by wordpress app to authenticate to mysqld.

### **wordpress-deployment.yaml**

Deploys wordpress app and creates a service to access the app.
PersistentVolumeClaim is created to persist wordpress website data files.


# Instructions

Instructions to deploy NDB Cluster and Wordpress.

## Deploy MySQL NDB Cluster in K8s

**Prerequisite**: NDB Operator is running.

```sh
# create a custom resource of type NdbCluster
kubectl apply -f wordpress-ndb.yaml

```

## Wait for the MySQL Cluster to start

```sh
# wait for the NdbCluster resource to become UpToDate
kubectl wait --for=condition=UpToDate ndb wordpress-ndb --timeout=10m

```

## Deploy wordpress app

```sh
# create a wordpress deployment
kubectl apply -f wordpress-deployment.yaml

```

This creates a wordpress app connected to mysqld created by ndb-operator.
A ClusterIP service is created to access the wordpress application deployed in K8s cluster.

## Expose wordpress services

Use the `kubectl port-forward` command to forward a port from the local machine to the port wordpress web app is listening inside the K8s node

```sh
# expose wordpress service port to local machine
kubectl port-forward service/wordpress 80:80

```

## Using wordpress

 Open a web browser and type in the IP address and port used with port-forward, in the search bar to start using the wordpress app.
