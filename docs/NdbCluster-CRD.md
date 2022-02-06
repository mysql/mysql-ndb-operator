# CRD Documentation
<p>Packages:</p>
<ul>
<li>
<a href="#mysql.oracle.com%2fv1alpha1">mysql.oracle.com/v1alpha1</a>
</li>
</ul>
<h2 id="mysql.oracle.com/v1alpha1">mysql.oracle.com/v1alpha1</h2>
<div>
<p>Package v1alpha1 is the v1alpha1 version of the API.</p>
</div>
Resource Types:
<ul><li>
<a href="#mysql.oracle.com/v1alpha1.NdbCluster">NdbCluster</a>
</li></ul>
<h3 id="mysql.oracle.com/v1alpha1.NdbCluster">NdbCluster
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
mysql.oracle.com/v1alpha1
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
<a href="#mysql.oracle.com/v1alpha1.NdbClusterSpec">NdbClusterSpec</a>
</em>
</td>
<td>
<i>Explained below</i>
</td>
</tr>
<tr>
<td>
<code>status</code><br/>
<em>
<a href="#mysql.oracle.com/v1alpha1.NdbClusterStatus">NdbClusterStatus</a>
</em>
</td>
<td>
<i>Explained below</i>
</td>
</tr>
</tbody>
</table>
<h3 id="mysql.oracle.com/v1alpha1.NdbClusterSpec">NdbClusterSpec
</h3>
<p>
(<em>Appears on:</em><a href="#mysql.oracle.com/v1alpha1.NdbCluster">NdbCluster</a>)
</p>
<div>
<p>NdbClusterSpec defines the desired state of MySQL Ndb Cluster</p>
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
<p>The number of data replicas or copies of data stored in Ndb Cluster.
Supported and allowed values are 1, 2, 3, and 4.
A redundancy level of 1 creates a sharded cluster providing
NO fault tolerance in case of node failure.
With a redundancy level of 2 or higher cluster will continue
serving client requests even in case of failures.
2 is the normal and most common value and the default.
A redundancy level of 3 provides additional protection.
For a redundancy level of 1 one management server will be created.
For 2 or higher two management servers will be used.
Once a cluster has been created, this number can NOT be easily changed.
More info :
<a href="https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-ndbd-definition.html#ndbparam-ndbd-noofreplicas">https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-ndbd-definition.html#ndbparam-ndbd-noofreplicas</a></p>
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
<p>The total number of data nodes in cluster.
The node count needs to be a multiple of the redundancyLevel.
Currently the maximum is 144 data nodes.</p>
</td>
</tr>
<tr>
<td>
<code>dataMemory</code><br/>
<em>
string
</em>
</td>
<td>
<p>DataMemory specifies the space available per data node
for storing in memory tables and indexes.
Allowed values 1M - 1T. More info :
<a href="https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-ndbd-definition.html#ndbparam-ndbd-datamemory">https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-ndbd-definition.html#ndbparam-ndbd-datamemory</a></p>
</td>
</tr>
<tr>
<td>
<code>extraNdbdDefaultParams</code><br/>
<em>
string
</em>
</td>
<td>
<em>(Optional)</em>
<p>Extra ndbd default parameters to be added to [ndbd default] section of config.ini file.
Exception: parameters DataMemory and NoOfReplicas should not be added as
they are handled through CRD attributes dataMemory and redundancyLevel. List of parameters :
<a href="https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-ndbd-definition.html">https://dev.mysql.com/doc/refman/8.0/en/mysql-cluster-ndbd-definition.html</a></p>
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
If not specified, &ldquo;mysql/mysql-cluster:latest&rdquo; will be used.
Lowest supported version is 8.0.26.</p>
</td>
</tr>
<tr>
<td>
<code>imagePullPolicy</code><br/>
<em>
<a href="https://v1-19.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#pullpolicy-v1-core">Kubernetes core/v1.PullPolicy</a>
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
<tr>
<td>
<code>dataNodePVCSpec</code><br/>
<em>
<a href="https://v1-19.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#persistentvolumeclaimspec-v1-core">Kubernetes core/v1.PersistentVolumeClaimSpec</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>DataNodePVCSpec is the PersistentVolumeClaimSpec to be used as the
VolumeClaimTemplate of the data node statefulset. A PVC will be created
for each data node by the statefulset controller and will be loaded into
the data node pod and the container.</p>
</td>
</tr>
<tr>
<td>
<code>mysqld</code><br/>
<em>
<a href="#mysql.oracle.com/v1alpha1.NdbMysqldSpec">NdbMysqldSpec</a>
</em>
</td>
<td>
<em>(Optional)</em>
</td>
</tr>
</tbody>
</table>
<h3 id="mysql.oracle.com/v1alpha1.NdbClusterStatus">NdbClusterStatus
</h3>
<p>
(<em>Appears on:</em><a href="#mysql.oracle.com/v1alpha1.NdbCluster">NdbCluster</a>)
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
<code>lastUpdate</code><br/>
<em>
<a href="https://v1-19.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#time-v1-meta">Kubernetes meta/v1.Time</a>
</em>
</td>
<td>
<p>LastUpdate is the time when the ProcessedGeneration was last updated.</p>
</td>
</tr>
<tr>
<td>
<code>receivedConfigHash</code><br/>
<em>
string
</em>
</td>
<td>
<p>The config hash of every new generation of a spec
received and acknowledged. Note : This is not used yet.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="mysql.oracle.com/v1alpha1.NdbMysqldSpec">NdbMysqldSpec
</h3>
<p>
(<em>Appears on:</em><a href="#mysql.oracle.com/v1alpha1.NdbClusterSpec">NdbClusterSpec</a>)
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
<p>NodeCount is the number of MySQL Servers running in MySQL Cluster</p>
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
</tbody>
</table>
<hr/>
