# Kubernetes Operator for MySQL NDB Cluster

This chart installs the Ndb CRD, deploys the Ndb Operator and the webhook server in the Kubernetes cluster.

## License

Copyright (c) 2021, Oracle and/or its affiliates.

License information can be found in the LICENSE file. This distribution may include materials developed by third parties. For license and attribution notices for these materials, please refer to the LICENSE file.

## Prerequisites

- Kubernetes 1.19+

## Installing the Chart

To install the chart with the release name `ndbop`:

```bash
helm install \
  --namespace=ndb-operator --create-namespace \
  ndbop deploy/charts/ndb-operator
```

The command creates the Ndb CRD, deploys the NDB Operator and the webhook server to the Kubernetes cluster with the default configuration. The [configuration](#configuration) section lists the parameters that can be configured during installation.

## Uninstalling the Chart

To uninstall/delete the `ndbop` release :

```bash
helm delete ndbop
```

The command removes the NDB Operator deployment and the webhook server deployment. The CRDs will not be deleted by this command. To remove them from the Kubernetes cluster :

```bash
kubectl delete customresourcedefinitions ndbclusters.mysql.oracle.com
```


## Configuration

The following table has the configurable parameters supported by the chart and their defaults.

| Parameter             | Description                         | Default                     |
| ----------------------| ------------------------------------| ----------------------------|
| `image`               | NDB Operator image name with tag    | `mysql/ndb-operator:latest` |
| `imagePullPolicy`     | NDB Operator image pull policy      | `IfNotPresent`              |
| `imagePullSecretName` | NDB Operator image pull secret name |                             |
