FROM oraclelinux:8-slim

# Just to make DNF happy
RUN touch /etc/dnf/dnf.conf

RUN microdnf install openssl-libs procps

RUN /usr/sbin/groupadd -g 27 -o -r mysql && \
    /usr/sbin/useradd -M -N -g mysql -o -r -s /bin/false -c "MySQL Server" -u 27 mysql

VOLUME /var/lib/ndb

COPY cluster-docker-files/sbin/* /usr/sbin/
COPY cluster-docker-files/bin/* /usr/bin/

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

