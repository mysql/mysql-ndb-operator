FROM mysql/mysql-cluster:8.0.22

RUN yum install -y procps

COPY c.ini /
COPY my.cnf /

ENTRYPOINT ["/bin/bash"]
