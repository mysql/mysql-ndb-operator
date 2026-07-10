# Management Node Access

The NDB management server protocol on TCP port 1186 is unauthenticated and can
stop, restart, or reconfigure a cluster. The NDB Operator therefore creates and
reconciles a managed `NetworkPolicy` for every `NdbCluster` by default, including
clusters that already exist when the operator is upgraded.

The policy allows TCP port 1186 only from:

- management, data, and MySQL pods labeled as components of the same `NdbCluster`
- same-namespace administrative pods labeled
  `mysql.oracle.com/allow-mgmd-access: <ndbcluster-name>`
- the NDB Operator pod selected by `app=ndb-operator` in the operator namespace

The operator recreates a deleted policy and restores changes to its selectors,
allowed peers, owner reference, required labels, protocol, and port.

## Security boundaries

NetworkPolicy enforcement depends on the Kubernetes network plugin.
If the CNI does not enforce `networking.k8s.io/v1` NetworkPolicy, the API object
will exist but traffic will not be restricted. Host-networked workloads and
node-originated traffic can also have CNI-specific behavior.

Kubernetes NetworkPolicies are additive. Another policy that selects the
management pods and allows broader ingress widens the effective access. Review
namespace-wide and allow-all policies before relying on the managed policy.

Pod labels are selectors, not authenticated identities. A principal that can
create pods in the cluster namespace can assign the reserved
`mysql.oracle.com/*` labels to those pods. Do not place mutually untrusted pod
creators in the same namespace as an `NdbCluster`; enforce that boundary with
namespace RBAC or an admission policy that reserves these labels. A labeled pod
in another namespace is not allowed by the managed policy.

## Administrative access

Label a trusted, same-namespace administrative pod before using `ndb_mgm` or
another management client:

```sh
kubectl label pod <pod-name> \
  mysql.oracle.com/allow-mgmd-access=<ndbcluster-name>
```

Remove the label after the administrative session. `kubectl port-forward` is
another option for authorized users, but its traffic may originate from a node
and must be controlled with Kubernetes RBAC rather than relying on NetworkPolicy.

## Upgrading existing installations

Existing clusters are protected automatically after the upgraded controller
reconciles them. Before upgrading, label every trusted management client as
shown above.

As a temporary compatibility escape hatch, an administrator can explicitly opt
an existing cluster out *before* upgrading:

```sh
kubectl annotate ndb <ndbcluster-name> \
  mysql.oracle.com/enable-management-network-policy=false
```

Missing or malformed values fail closed and enable the policy. A new cluster is
always mutated to `true`. After protection is enabled, changing the annotation
to `false` is rejected. Set an opted-out cluster to `true` after its clients are
ready:

```sh
kubectl annotate ndb <ndbcluster-name> --overwrite \
  mysql.oracle.com/enable-management-network-policy=true
```

The `ManagementAccessProtected` status condition is `False` while the policy is
explicitly disabled or a legacy management LoadBalancer remains enabled. This
condition reports operator configuration; it cannot verify CNI enforcement or
detect a separate permissive NetworkPolicy.

During an operator upgrade the webhook and controller Deployments use the
`Recreate` strategy. NdbCluster admission fails closed briefly while the webhook
restarts, and mixed controller versions never reconcile the same objects.

## Management LoadBalancers

`spec.managementNode.enableLoadBalancer` is deprecated. Creates and updates that
newly enable it are rejected.

An already-existing management LoadBalancer is preserved while the deprecated
field remains `true`, but warning logs, Events, and the status condition identify
the exposure. The setting is also honored when the Service must be recreated so
legacy external clients continue working after recovery. Set the field to `false`
to convert the Service to `ClusterIP`; admission prevents enabling it again.
