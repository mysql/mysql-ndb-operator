FROM oraclelinux:8-slim

ARG MYSQL_CLUSTER_PACKAGE_URL=https://repo.mysql.com/yum/mysql-cluster-8.0-community/docker/x86_64/mysql-cluster-community-server-minimal-8.0.18-0.1.rc.el7.x86_64.rpm
#mysql-cluster-community-server-minimal-8.0.19-1.el7.x86_64.rpm

# using --nodeps to avoid unnecessary per install
RUN rpmkeys --import https://repo.mysql.com/RPM-GPG-KEY-mysql && \ 
    microdnf install -y iputils && \
    microdnf install vim && \
    microdnf install gcc bison cmake && \
    microdnf clean all 

ENTRYPOINT ["/bin/bash"]

