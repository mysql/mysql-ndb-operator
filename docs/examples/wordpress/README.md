# Wordpress on NDB

Contains resource files to run a wordpress app in K8s utilizing ndb resources
created using ndb-operator.

## License

Copyright (c) 2021, Oracle and/or its affiliates.

License information can be found in the LICENSE file. This distribution may include materials developed by third parties. For license and attribution notices for these materials, please refer to the LICENSE file.

## Resource files details

### **wordpress-ndb.yaml**

Deploys ndb cluster with two ndbd, two mysqld and two mgmd(defaut).
Services to access ndb cluster is created. Generates a "secret"
that will be used by wordpress app to authenticate to mysqld.

### **wordpress-deployment.yaml**

Deploys wordpress app and creates a service to access the app.
PersistentVolumeClaim is created to persist wordpress website data files.


# Instructions

Instructions to deploy NDB Cluster and Wordpress.

## Deploy ndb cluster in K8s

**Prerequisite**: ndb operator is running.

```sh
# create a custom resource of type Ndb
kubectl apply -f wordpress-ndb.yaml

```

## Expose K8s services (Optional)

**Note**: If minikube is used, make sure you have created a tunnel, to ensure
          K8s services are externally available.

## Check readiness of ndb cluster

```sh

# retrieve mgmd LoadBalancer service IP  using the service name
MGMD_SERVICE=$(kubectl get service "wordpress-ndb-mgmd-ext" \
  -o jsonpath={.status.loadBalancer.ingress[0].ip})

# check cluster status using ndb_mgm's 'show' command
ndb_mgm -c ${MGMD_SERVICE} -e "show"

```

Ensure all ndb cluster nodes are connected to mgmd.

## Deploy wordpress app

**Prerequisite**: [Check readiness of ndb cluster](#check-readiness-of-ndb-cluster)

```sh
# create a wordpress deployment
kubectl apply -f wordpress-deployment.yaml

```

This creates a wordpress app connected to mysqld created by ndb-operator.
A LoadBalancer service is created to access the wordpress application
deployed in K8s cluster.

## Using wordpress

```sh
# retrieve wordpress LoadBalancer service IP address using the service name
kubectl get service "wordpress" \
 -o jsonpath={.status.loadBalancer.ingress[0].ip}

 ```
 Open a web browser and type in the IP address obtained, in the search bar
 to start using the wordpress app.
