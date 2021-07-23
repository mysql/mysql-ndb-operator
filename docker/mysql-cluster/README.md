## Build MySQL Cluster docker image

Ndb operator uses the public images available in dockerhub. By default, the mysql/mysql-cluster:latest image is used. A custom MySQL Cluster image can also be built and used with the operator. The docker files and scripts required for building a MySQL Cluster image are included in this directory.

### License

Copyright (c) 2021, Oracle and/or its affiliates.

License information can be found in the LICENSE file. This distribution may include materials developed by third parties. For license and attribution notices for these materials, please refer to the LICENSE file.

### Build image from source code

To compile and build a custom image from source code run,
```sh
# SRCDIR is the location of the MySQL Cluster Source
# TMPDIR is the location in which the MySQL Cluster will be built
# IMAGE_TAG is the string to be tagged to the container image being built
SRCDIR=/path/to/mysql/cluster/source TMPDIR=/build/dir IMAGE_TAG=<custom-tag> build.sh
```

Note : To compile and build the image directly on docker hosted inside minikube, additional steps might be required. Please look at [docker/mysql-cluster/build.sh](docker/mysql-cluster/build.sh) file for more information.

### Build image from precompiled binaries

To build a custom image from precompiled binaries for Oracle Linux 8,
```sh
# BASEDIR is the path to the MySQL Cluster OL8 build or install location.
# IMAGE_TAG is the optional string to be tagged to the container image being built
BASEDIR=/path/to/build/or/install/dir IMAGE_TAG=<custom-tag> build.sh
```
