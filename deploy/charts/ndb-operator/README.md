
# Kubernetes Operator for MySQL NDB Cluster

This chart installs the NdbCluster CRD, deploys the Ndb Operator and the webhook server in the Kubernetes cluster.

## License

Copyright (c) 2021, Oracle and/or its affiliates.

License information can be found in the LICENSE file. This distribution may include materials developed by third parties. For license and attribution notices for these materials, please refer to the LICENSE file.

## Prerequisites

- Kubernetes v1.19.0+

## Installing the Chart

To install the chart with the release name `ndbop`:

```bash
helm install \
  --namespace=ndb-operator --create-namespace \
  ndbop deploy/charts/ndb-operator
```

The command creates the NdbCluster CRD, deploys the NDB Operator and the webhook server to the Kubernetes cluster with the default configuration in the `ndb-operator` namespace.
The [configuration](#configuration) section lists the configurable options provided by the helm chart.

## Uninstalling the Chart

To uninstall/delete the `ndbop` release :

```bash
helm uninstall --namespace=ndb-operator ndbop
```
The command `helm uninstall` removes only the NDB Operator deployment and the webhook server deployment. The NdbCluster CRD will not be deleted by this command.

To delete the CRD from the Kubernetes cluster :

```bash
kubectl delete customresourcedefinitions ndbclusters.mysql.oracle.com
```
Note that removing the NdbCluster CRD will also stop and delete any MySQL Cluster pods started by the NDB Operator.

## Configuration

The following table has the configurable options supported by the chart and their defaults.

| Parameter             | Description                         | Default                     |
| ----------------------| ------------------------------------| ----------------------------|
| `image`               | NDB Operator image name with tag    | `mysql/ndb-operator:latest` |
| `imagePullPolicy`     | NDB Operator image pull policy      | `IfNotPresent`              |
| `imagePullSecretName` | NDB Operator image pull secret name |                             |
| `clusterScoped`       | Scope of the Ndb Operator.<br>If `true`, the operator is cluster-scoped and will watch for changes to any NdbCluster resource across all namespaces.<br>If `false`, the operator is namespace-scoped and will only watch for changes in the namespace it is released into. | `true`|

These options can be set using the '–set' argument of the helm CLI.

For example, to specify a custom imagePullPolicy,
```bash
helm install --set imagePullPolicy=Always ndbop deploy/charts/ndb-operator
```
