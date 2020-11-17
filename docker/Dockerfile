FROM oraclelinux:8-slim

#ARG BASENAME=mysql-cluster-community
#ARG VERSION=8.0.22-1.tr.el8.x86_64
#ARG PB2BUILD=38605488

RUN microdnf install openssl-libs 

# handy to have ps
RUN microdnf install procps

# toggle the two "ADD" lines if you want to fetch
# or fetch with wget obviously
# ADD runs tar x automatically
#ADD http://vanheim.no.oracle.com/archive/${PB2BUILD}.${BASENAME}-${VERSION}.tar /root/SOURCE/rpm
#ADD vendor/${PB2BUILD}.${BASENAME}-${VERSION}.tar /root/SOURCE/rpm

# microdnf does not uinstall files - deviate to rpm
#RUN RPMS="common client data-node libs management-server ndbclient server"; \
#    for i in $RPMS; do \
#        rpm -i /root/SOURCE/rpm/${BASENAME}-${i}-${VERSION}.rpm; \
#    done; \
#    rm -rf /root/SOURCE/rpm

#TODO make caching friendly
ENV CONFD_VERSION=0.16.0
#ADD https://github.com/kelseyhightower/confd/releases/download/v${CONFD_VERSION}/confd-${CONFD_VERSION}-linux-amd64 /usr/bin/confd

# we use a self build version on go 1.11+ which supports variable overwrite
COPY 3rdparty/confd-${CONFD_VERSION}-linux-amd64 /usr/bin/confd
RUN chmod a+rx /usr/bin/confd


RUN /usr/sbin/groupadd -g 27 -o -r mysql
RUN /usr/sbin/useradd -M -N -g mysql -o -r -s /bin/false \
    -c "MySQL Server" -u 27 mysql 

VOLUME /var/lib/ndb

COPY bin/mysql/sbin/ndbmtd /usr/sbin/
COPY bin/mysql/sbin/ndb_mgmd /usr/sbin/
COPY bin/mysql/sbin/mysqld /usr/sbin/
COPY bin/mysql/sbin/mysqladmin /usr/sbin/

COPY bin/mysql/bin/ndb_mgm /usr/bin/
COPY bin/mysql/bin/mysql /usr/bin/

COPY docker/docker-entrypoint.sh /entrypoint.sh
COPY docker/cnf/my.cnf /etc/
COPY docker/cnf/ndb-cluster.toml /etc/confd/conf.d/
COPY docker/cnf/ndb-cluster.cnf.tmpl /etc/confd/templates/


EXPOSE 3306 33060 1186 8080

# https://click.palletsprojects.com/en/7.x/python3/#python3-surrogates
ENV LC_ALL de_DE.utf-8
ENV LANG de_DE.utf-8

#USER mysql

ENTRYPOINT ["/entrypoint.sh"]
CMD ["ndb_mgmd"]

