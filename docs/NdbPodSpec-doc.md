# Define pod specifications using NdbPodSpec

The NdbCluster CRD provides the [NdbPodSpec](NdbCluster-CRD.md#ndbpodspec) structure to mention pod specifications. Pod specifications for Management, Data and MySQL node pods can be individually defined via their respective NdbPodSpec fields - `.spec.managementNode.ndbPodSpec`, `.spec.dataNode.ndbPodSpec` and `.spec.mysqlNode.ndbPodSpec`. The values filled into these NdbPodSpec fields will be copied into the respective StatefulSet definitions. This document explains how these NdbPodSpec fields can be used to assign pods to particular worker nodes, define affinity rules and to specify pod resource requirements.

## Assigning MySQL Cluster node pods to desired worker pods

NdbPodSpec has a `nodeSelector` field through which one can specify the labels of the Worker Node on which that particular MySQL Cluster node should be scheduled. The document at [assign-pod-node](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node) has more information on how nodeSelector works and how to manually attach labels to worker nodes.

## Affinity and anti-affinity

NdbPodSpec has an `affinity` field through which the affinity/anti-affinity rules for the respective MySQL Cluster nodes can be defined. The K8s Cluster uses these rules to decide where to or where not to schedule a MySQL Cluster pod. Please note that these rules are applied after filtering out the available worker node pool based on any specified `nodeSelector` labels. For more information on how Kubernetes uses the affinity/anti-affinity rules to schedule pods, please read - [affinity and anti-affinity](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#affinity-and-anti-affinity).

The NDB Operator defines default anti-affinity rules for all three MySQL Cluster node types (i.e. mgmd, ndbmtd, mysqld) to prevent them from being scheduled unto the same worker node whenever possible. These default anti-affinity rules are defined as a `preferredDuringSchedulingIgnoredDuringExecution` rule, i.e. they will be satisfied by the K8s scheduler only if there are enough resources. For example, if 4 data nodes have to be scheduled on 4 worker nodes, each data node will be scheduled on a separate worker node but if 6 data nodes have to be scheduled on 4 worker nodes, the first 4 data nodes will be scheduled on 4 separate worker nodes and the 5th and 6th data nodes have to be scheduled on worker nodes where a data node is already running. The default anti-affinity rules can be overridden by specifying the desired anti-affinity rules via the `affinity` field.

## Specify Resource Requirements

The resource requirements for the pods can be specified using the `resources` field of the respective MySQL Cluster nodes' ndbPodSpec. These resources requirements will be copied into the respective container definitions. For more information read [Resource Management for Pods and Containers](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/).

The NDB Operator defines default memory requirements for the MySQL Cluster data nodes based on the configuration of the MySQL Cluster. The minimum memory requirement calculated by the NDB Operator for this purpose is mostly a guesstimate and is provided only to filter out obvious bad choice worker nodes. This default value can be overrided by specifying an alternative via data nodes's ndbPodSpec.
