# Kubernetes Operator for MySQL NDB Cluster

This chart installs the NdbCluster CRD, deploys the Ndb Operator and the webhook server in the Kubernetes cluster.

## License

Copyright (c) 2021, 2025, Oracle and/or its affiliates.

License information can be found in the LICENSE file. This distribution may include materials developed by third parties. For license and attribution notices for these materials, please refer to the LICENSE file.

## Prerequisites

- Kubernetes v1.23.0+

## Install the Chart

The `helm install` command can be used to create the NdbCluster CRD, deploy the NDB Operator and the webhook server to the Kubernetes Cluster.
The [configuration](#configuration) section lists the configurable options provided by the helm chart.

### Install from helm repo

The NDB Operator helm repository is hosted at `https://mysql.github.io/mysql-ndb-operator/`.

To add the chart repository :

```bash
helm repo add ndb-operator-repo https://mysql.github.io/mysql-ndb-operator/
helm repo update
```

To install the chart with the release name `ndbop`:

```bash
helm install \
  --namespace=ndb-operator --create-namespace \
  ndbop ndb-operator-repo/ndb-operator
```

### Install from source code

To install the chart from the source code, with the release name `ndbop`:

```bash
helm install \
  --namespace=ndb-operator --create-namespace \
  ndbop deploy/charts/ndb-operator
```

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

The following table lists the configurable options supported by the chart, along with their default values.

| Parameter             | Description                                                                                                                                                                                                                                 | Default                     |
| ----------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------|
| `image`               | The name of the NDB Operator image, including its tag.                                                                                                                                                                                      | `container-registry.oracle.com/mysql/community-ndb-operator:9.4.0-1.9.0` |
| `imagePullPolicy`     | The image pull policy for the NDB Operator.                                                                                                                                                                                                 | `IfNotPresent`              |
| `imagePullSecretName` | The name of the secret used to authenticate image pulls for the NDB Operator.                                                                                                                                                               | None                        |
| `clusterScoped`       | Determines the scope of the NDB Operator.<br>When set to `true`, the operator watches for changes to any NdbCluster resource across all namespaces.<br>When set to `false`, the operator only watches for changes within its own namespace. | `true`                      |
| `watchNamespace`      | Specifies the namespace to monitor for NdbCluster resource changes.<br>This option is only applicable when `clusterScoped` is set to `false`. If not specified, the operator will only watch the namespace where it is deployed.            | None                        |

These options can be set using the '–set' argument of the Helm CLI.

For example, to specify a custom imagePullPolicy,
```bash
helm install --set imagePullPolicy=Always ndbop deploy/charts/ndb-operator
```

### Security

The following table lists the configurable security options supported by the chart, along with their default values.

| Parameter             | Description                                                                                                                                                                                                                                 | Default                     |
| ----------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------|
| `operatorRunAsUser` | Sets the UID of the NDB Operator processes. This should be left empty when the K8S platform automatically assigns the UID to the processes. | `27` |
| `operatorRunAsGroup` | Sets the GID of the NDB Operator processes. This should be left empty when the K8S platform automatically assigns the GID to the processes. | `27` |
| `enableSecurityContext` | Enables the deployment of NDB pods with a stricter security context. | `false` |
| `usePlatformAssignedIDs` | Allows the K8S platform to automatically assign the UID and GID to the NDB Cluster processes. This should only be used when the target K8S platform supports automatic assignment. | `false` |
| `runAsUser` | Sets the UID of the NDB Cluster processes. | `27` |
| `runAsGroup` | Sets the GID of the NDB Cluster processes. | `27` |
| `fsGroup` | Sets the FS Group of the mounted partitions. This is useful when using persistent volumes to store data. | `27` |
