Contributing Guidelines
====================
We welcome your code contributions. Before submitting code via a GitHub pull
request, or by filing a bug in https://bugs.mysql.com you will need to have
signed the Oracle Contributor Agreement (see https://oca.opensource.oracle.com).

Only pull requests from committers that can be verified as having signed the OCA
can be accepted.

Submitting a contribution
--------------------------
In order to contribute to MySQL NDB Operator, you need to follow these steps:

1. Make sure you have a user account at https://bugs.mysql.com. You'll need to reference this
   user account when you submit your OCA (Oracle Contributor Agreement).
2. Sign the Oracle OCA. You can find instructions for doing that at the OCA Page,
   at https://oca.opensource.oracle.com
3. Validate your contribution by including tests that sufficiently cover the functionality.
4. Verify that the entire test suite passes with your code applied.
5. Submit your pull request via [GitHub](https://github.com/mysql/mysql-ndb-operator/pulls) or 
   uploading it using the contribution tab to a bug record in https://bugs.mysql.com (using 
   the 'contribution' tab).

Reporting Issues and Requesting Enhancements
--------------------------
You can report issues and request enhancements through the 
[MySQL bug system](https://bugs.mysql.com/report.php?in%5Bbug_type%5D=Server%3A+Cluster_OPR).

Before reporting a new bug, please [check first](https://bugs.mysql.com/search.php?bug_type%5B%5D=Server%3A+Cluster_OPR) 
to see if a similar bug already exists.

Bug reports should be as complete as possible. Please try and include the following:

* Complete steps to reproduce the issue;
* Any information about the K8s environment that could be specific to the bug;
* Specific version of the MySQL NDB Operator being;
* Specific version of the MySQL NDB Cluster being used;
* Sample code to help reproduce the issue, if possible.