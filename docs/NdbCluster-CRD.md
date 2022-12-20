# CRD Documentation
<p>Packages:</p>
<ul>
<li>
<a href="#mysql.oracle.com%2fv1">mysql.oracle.com/v1</a>
</li>
</ul>
<h2 id="mysql.oracle.com/v1">mysql.oracle.com/v1</h2>
<div>
<p>Package v1 is the v1 version of the API.</p>
</div>
Resource Types:
<ul><li>
<a href="#mysql.oracle.com/v1.NdbCluster">NdbCluster</a>
</li></ul>
<h3 id="mysql.oracle.com/v1.NdbCluster">NdbCluster
</h3>
<div>
<p>NdbCluster is the Schema for the Ndb CRD API</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code><br/>
string</td>
<td>
<code>
mysql.oracle.com/v1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code><br/>
string
</td>
<td><code>NdbCluster</code></td>
</tr>
<tr>
<td>
<code>spec</code><br/>
<em>
<a href="#mysql.oracle.com/v1.NdbClusterSpec">NdbClusterSpec</a>
</em>
</td>
<td>
<p>The desired state of a MySQL NDB Cluster.</p>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#mysql.oracle.com/v1.NdbClusterStatus">NdbClusterStatus</a>
</em>
</td>
<td>
<p>The status of the NdbCluster resource and the MySQL Cluster managed by it.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="mysql.oracle.com/v1.NdbClusterCondition">NdbClusterCondition
</h3>
<p>
(<em>Appears on:</em><a href="#mysql.oracle.com/v1.NdbClusterStatus">NdbClusterStatus</a>)
</p>
<div>
<p>NdbClusterCondition describes the state of a MySQL Cluster installation at a certain point.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>type</code><br/>
<em>
<a href="#mysql.oracle.com/v1.NdbClusterConditionType">NdbClusterConditionType</a>
</em>
</td>
<td>
<p>Type of NdbCluster condition.</p>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="https://pkg.go.dev/k8s.io/api@v0.20.2/core/v1#ConditionStatus">Kubernetes core/v1.ConditionStatus</a>
</em>
</td>
<td>
<p>Status of the condition, one of True, False, Unknown.</p>
</td>
</tr>
<tr>
<td>
<code>lastTransitionTime</code><br/>
<em>
<a href="https://pkg.go.dev/k8s.io/api@v0.20.2/meta/v1#Time">Kubernetes meta/v1.Time</a>
</em>
</td>
<td>
<p>Last time the condition transitioned from one status to another.</p>
</td>
</tr>
<tr>
<td>
<code>reason</code><br/>
<em>
string
</em>
</td>
<td>
<p>The reason for the condition&rsquo;s last transition.</p>
</td>
</tr>
<tr>
<td>
<code>message</code><br/>
<em>
string
</em>
</td>
<td>
<p>A human-readable message indicating details about the transition.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="mysql.oracle.com/v1.NdbClusterConditionType">NdbClusterConditionType
(<code>string</code> alias)</h3>
<p>
(<em>Appears on:</em><a href="#mysql.oracle.com/v1.NdbClusterCondition">NdbClusterCondition</a>)
</p>
<div>
<p>NdbClusterConditionType defines type for NdbCluster condition.</p>
</div>
<table>
<thead>
<tr>
<th>Value</th>
<th>Description</th>
</tr>
</thead>
<tbody><tr><td><p>&#34;UpToDate&#34;</p></td>
<td><p>NdbClusterUpToDate specifies if the spec of the MySQL Cluster
is up-to-date with the NdbCluster resource spec</p>
</td>
</tr></tbody>
</table>
<h3 id="mysql.oracle.com/v1.NdbClusterPodSpec">NdbClusterPodSpec
</h3>
<p>
(<em>Appears on:</em><a href="#mysql.oracle.com/v1.NdbDataNodeSpec">NdbDataNodeSpec</a>, <a href="#mysql.oracle.com/v1.NdbManagementNodeSpec">NdbManagementNodeSpec</a>, <a href="#mysql.oracle.com/v1.NdbMysqldSpec">NdbMysqldSpec</a>)
</p>
<div>
<p>NdbClusterPodSpec contains a subset of PodSpec fields which when set
will be copied into to the podSpec of respective MySQL Cluster
node workload definitions.</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>resources</code><br/>
<em>
<a href="https://pkg.go.dev/k8s.io/api@v0.20.2/core/v1#ResourceRequirements">Kubernetes core/v1.ResourceRequirements</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Total compute Resources required by this pod.
Cannot be updated.</p>
<p>More info: <a href="https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/">https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/</a></p>
</td>
</tr>
<tr>
<td>
<code>nodeSelector</code><br/>
<em>
map[string]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>NodeSelector is a selector which must be true for the pod to fit on a node.
Selector which must match a node&rsquo;s labels for the pod to be scheduled on that node.</p>
<p>More info: <a href="https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#nodeselector">https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#nodeselector</a></p>
</td>
</tr>
<tr>
<td>
<code>affinity</code><br/>
<em>
<a href="https://pkg.go.dev/k8s.io/api@v0.20.2/core/v1#Affinity">Kubernetes core/v1.Affinity</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>If specified, the pod&rsquo;s scheduling constraints</p>
</td>
</tr>
<tr>
<td>
<code>schedulerName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>If specified, the pod will be dispatched by specified scheduler.
If not specified, the pod will be dispatched by default scheduler.</p>
</td>
</tr>
<tr>
<td>
<code>tolerations</code><br/>
<em>
<a href="https://pkg.go.dev/k8s.io/api@v0.20.2/core/v1#Toleration">[]Kubernetes core/v1.Toleration</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>If specified, the pod&rsquo;s tolerations.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="mysql.oracle.com/v1.NdbClusterSpec">NdbClusterSpec
</h3>
<p>
(<em>Appears on:</em><a href="#mysql.oracle.com/v1.NdbCluster">NdbCluster</a>)
</p>
<div>
<p>NdbClusterSpec defines the desired state of a MySQL NDB Cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>redundancyLevel</code><br/>
<em>
int32
</em>
</td>
<td>
<em>(Optional)</em>
<p>The number of copies of all data stored in MySQL Cluster.
This also defines the number of nodes in a node group.
Supported values are 1, 2, 3, and 4.
Note that, setting this to 1 means that there is only a
single copy of all MySQL Cluster data and failure of any
Data node will cause the entire MySQL Cluster to fail.
The operator also implicitly decides the number of
Management nodes to be added to the MySQL Cluster
configuration based on this value. For a redundancy level
of 1, one Management node will be created. For 2 or
higher, two Management nodes will be created.
This value is immutable.</p>
<p>More info :
<a href="https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-ndbd-definition.html#ndbparam-ndbd-noofreplicas">https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-ndbd-definition.html#ndbparam-ndbd-noofreplicas</a></p>
</td>
</tr>
<tr>
<td>
<code>managementNode</code><br/>
<em>
<a href="#mysql.oracle.com/v1.NdbManagementNodeSpec">NdbManagementNodeSpec</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ManagementNode specifies the configuration of the management node running in MySQL Cluster.</p>
</td>
</tr>
<tr>
<td>
<code>dataNode</code><br/>
<em>
<a href="#mysql.oracle.com/v1.NdbDataNodeSpec">NdbDataNodeSpec</a>
</em>
</td>
<td>
<p>DataNode specifies the configuration of the data node running in MySQL Cluster.</p>
</td>
</tr>
<tr>
<td>
<code>mysqlNode</code><br/>
<em>
<a href="#mysql.oracle.com/v1.NdbMysqldSpec">NdbMysqldSpec</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>MysqlNode specifies the configuration of the MySQL Servers running in the cluster.
Note that the NDB Operator requires atleast one MySQL Server running in the cluster
for internal operations. If no MySQL Server is specified, the operator will by
default add one MySQL Server to the spec.</p>
</td>
</tr>
<tr>
<td>
<code>freeAPISlots</code><br/>
<em>
int32
</em>
</td>
<td>
<em>(Optional)</em>
<p>The number of extra API sections declared in the MySQL Cluster
config, in addition to the API sections declared implicitly
by the NDB Operator for the MySQL Servers.
Any NDBAPI application can connect to the MySQL Cluster via
these free slots.</p>
</td>
</tr>
<tr>
<td>
<code>image</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The name of the MySQL Ndb Cluster image to be used.
If not specified, &ldquo;mysql/mysql-cluster:8.0.32&rdquo; will be used.</p>
</td>
</tr>
<tr>
<td>
<code>imagePullPolicy</code><br/>
<em>
<a href="https://pkg.go.dev/k8s.io/api@v0.20.2/core/v1#PullPolicy">Kubernetes core/v1.PullPolicy</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>ImagePullPolicy describes a policy for if/when to
pull the MySQL Cluster container image</p>
</td>
</tr>
<tr>
<td>
<code>imagePullSecretName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>ImagePullSecretName optionally specifies the name of the secret that
holds the credentials required for pulling the MySQL Cluster image.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="mysql.oracle.com/v1.NdbClusterStatus">NdbClusterStatus
</h3>
<p>
(<em>Appears on:</em><a href="#mysql.oracle.com/v1.NdbCluster">NdbCluster</a>)
</p>
<div>
<p>NdbClusterStatus is the status for a Ndb resource</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>processedGeneration</code><br/>
<em>
int64
</em>
</td>
<td>
<p>ProcessedGeneration holds the latest generation of the
Ndb resource whose specs have been successfully applied
to the MySQL Cluster running inside K8s.</p>
</td>
</tr>
<tr>
<td>
<code>readyManagementNodes</code><br/>
<em>
string
</em>
</td>
<td>
<p>The status of the MySQL Cluster Management nodes.</p>
</td>
</tr>
<tr>
<td>
<code>readyDataNodes</code><br/>
<em>
string
</em>
</td>
<td>
<p>The status of the MySQL Cluster Data nodes.</p>
</td>
</tr>
<tr>
<td>
<code>readyMySQLServers</code><br/>
<em>
string
</em>
</td>
<td>
<p>The status of the MySQL Servers.</p>
</td>
</tr>
<tr>
<td>
<code>conditions</code><br/>
<em>
<a href="#mysql.oracle.com/v1.NdbClusterCondition">[]NdbClusterCondition</a>
</em>
</td>
<td>
<p>Conditions represent the latest available
observations of the MySQL Cluster&rsquo;s current state.</p>
</td>
</tr>
<tr>
<td>
<code>generatedRootPasswordSecretName</code><br/>
<em>
string
</em>
</td>
<td>
<p>GeneratedRootPasswordSecretName is the name of the secret generated by the
operator to be used as the MySQL Server root account password. This will
be set to nil if a secret has been already provided to the operator via
spec.mysqlNode.rootPasswordSecretName.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="mysql.oracle.com/v1.NdbDataNodeSpec">NdbDataNodeSpec
</h3>
<p>
(<em>Appears on:</em><a href="#mysql.oracle.com/v1.NdbClusterSpec">NdbClusterSpec</a>)
</p>
<div>
<p>NdbDataNodeSpec is the specification of data node in MySQL Cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>config</code><br/>
<em>
map[string]*<a href="https://pkg.go.dev/k8s.io/apimachinery/pkg/util/intstr#IntOrString">Kubernetes util/intstr.IntOrString</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Config is a map of default MySQL Cluster Data node configurations.</p>
<p>More info :
<a href="https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-params-ndbd.html">https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-params-ndbd.html</a></p>
</td>
</tr>
<tr>
<td>
<code>ndbPodSpec</code><br/>
<em>
<a href="#mysql.oracle.com/v1.NdbClusterPodSpec">NdbClusterPodSpec</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>NdbPodSpec contains a subset of PodSpec fields which when
set will be copied into to the podSpec of Data node&rsquo;s statefulset
definition.</p>
</td>
</tr>
<tr>
<td>
<code>nodeCount</code><br/>
<em>
int32
</em>
</td>
<td>
<p>The total number of data nodes in MySQL Cluster.
The node count needs to be a multiple of the
redundancyLevel. A maximum of 144 data nodes are
allowed to run in a single MySQL Cluster.</p>
</td>
</tr>
<tr>
<td>
<code>pvcSpec</code><br/>
<em>
<a href="https://pkg.go.dev/k8s.io/api@v0.20.2/core/v1#PersistentVolumeClaimSpec">Kubernetes core/v1.PersistentVolumeClaimSpec</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>PVCSpec is the PersistentVolumeClaimSpec to be used as the
VolumeClaimTemplate of the data node statefulset. A PVC will be created
for each data node by the statefulset controller and will be loaded into
the data node pod and the container.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="mysql.oracle.com/v1.NdbManagementNodeSpec">NdbManagementNodeSpec
</h3>
<p>
(<em>Appears on:</em><a href="#mysql.oracle.com/v1.NdbClusterSpec">NdbClusterSpec</a>)
</p>
<div>
<p>NdbManagementNodeSpec is the specification of management node in MySQL Cluster</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>config</code><br/>
<em>
map[string]*<a href="https://pkg.go.dev/k8s.io/apimachinery/pkg/util/intstr#IntOrString">Kubernetes util/intstr.IntOrString</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>Config is a map of default MySQL Cluster Management node configurations.</p>
<p>More info :
<a href="https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-params-mgmd.html">https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-params-mgmd.html</a></p>
</td>
</tr>
<tr>
<td>
<code>ndbPodSpec</code><br/>
<em>
<a href="#mysql.oracle.com/v1.NdbClusterPodSpec">NdbClusterPodSpec</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>NdbPodSpec contains a subset of PodSpec fields which when
set will be copied into to the podSpec of Management node&rsquo;s
statefulset definition.</p>
</td>
</tr>
<tr>
<td>
<code>enableLoadBalancer</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>EnableLoadBalancer exposes the management servers externally using the
kubernetes cloud provider&rsquo;s load balancer. By default, the operator creates a ClusterIP
type service to expose the management server pods internally within the kubernetes cluster.
If EnableLoadBalancer is set to true, a LoadBalancer type service will be created instead,
exposing the management Servers outside the kubernetes cluster.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="mysql.oracle.com/v1.NdbMysqldSpec">NdbMysqldSpec
</h3>
<p>
(<em>Appears on:</em><a href="#mysql.oracle.com/v1.NdbClusterSpec">NdbClusterSpec</a>)
</p>
<div>
<p>NdbMysqldSpec is the specification of MySQL Servers to be run as an SQL Frontend</p>
</div>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>nodeCount</code><br/>
<em>
int32
</em>
</td>
<td>
<p>NodeCount is the number of MySQL Servers to be started by the Operator</p>
</td>
</tr>
<tr>
<td>
<code>maxNodeCount</code><br/>
<em>
int32
</em>
</td>
<td>
<em>(Optional)</em>
<p>MaxNodeCount is the count up to which the MySQL Servers would be
allowed to scale up without forcing a MySQL Cluster config update.
If unspecified, operator will define the MySQL Cluster config with
API sections for two additional MySQL Servers.</p>
</td>
</tr>
<tr>
<td>
<code>connectionPoolSize</code><br/>
<em>
int32
</em>
</td>
<td>
<em>(Optional)</em>
<p>ConnectionPoolSize is the number of connections a single
MySQL Server should use to connect to the MySQL Cluster nodes.
More info :
<a href="https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-options-variables.html#option_mysqld_ndb-cluster-connection-pool">https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-options-variables.html#option_mysqld_ndb-cluster-connection-pool</a></p>
</td>
</tr>
<tr>
<td>
<code>rootPasswordSecretName</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>The name of the Secret that holds the password to be set for the MySQL
root accounts. The Secret should have a &lsquo;password&rsquo; key that holds the
password.
If unspecified, a Secret will be created by the operator with a generated
name of format &ldquo;&lt;ndb-resource-name&gt;-mysqld-root-password&rdquo;</p>
</td>
</tr>
<tr>
<td>
<code>rootHost</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>RootHost is the host or hosts from which the root user
can connect to the MySQL Server. If unspecified, root user
will be able to connect from any host that can access the MySQL Server.</p>
</td>
</tr>
<tr>
<td>
<code>myCnf</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Configuration options to pass to the MySQL Server when it is started.</p>
</td>
</tr>
<tr>
<td>
<code>enableLoadBalancer</code><br/>
<em>
bool
</em>
</td>
<td>
<em>(Optional)</em>
<p>EnableLoadBalancer exposes the MySQL servers externally using the kubernetes cloud
provider&rsquo;s load balancer. By default, the operator creates a ClusterIP type service
to expose the MySQL server pods internally within the kubernetes cluster. If
EnableLoadBalancer is set to true, a LoadBalancer type service will be created instead,
exposing the MySQL servers outside the kubernetes cluster.</p>
</td>
</tr>
<tr>
<td>
<code>ndbPodSpec</code><br/>
<em>
<a href="#mysql.oracle.com/v1.NdbClusterPodSpec">NdbClusterPodSpec</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>NdbPodSpec contains a subset of K8s PodSpec fields which when set
will be copied into to the podSpec of MySQL Server StatefulSet.</p>
</td>
</tr>
<tr>
<td>
<code>initScripts</code><br/>
<em>
map[string][]string
</em>
</td>
<td>
<em>(Optional)</em>
<p>InitScripts is a map of configMap names from the same namespace and
optionally an array of keys which store the SQL scripts to be executed
during MySQL Server initialization. If key names are omitted, contents
of all the keys will be treated as initialization SQL scripts. All
scripts will be mounted into the MySQL pods and will be executed in the
alphabetical order of configMap names and key names.</p>
</td>
</tr>
<tr>
<td>
<code>pvcSpec</code><br/>
<em>
<a href="https://pkg.go.dev/k8s.io/api@v0.20.2/core/v1#PersistentVolumeClaimSpec">Kubernetes core/v1.PersistentVolumeClaimSpec</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>PVCSpec is the PersistentVolumeClaimSpec to be used as the
VolumeClaimTemplate of the mysql server statefulset. A PVC will be created
for each mysql server by the statefulset controller and will be loaded into
the mysql server pod and the container.</p>
</td>
</tr>
</tbody>
</table>
<hr/>
