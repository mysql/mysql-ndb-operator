FROM oraclelinux:8-slim

RUN /usr/sbin/groupadd -g 27 -o -r mysql
RUN /usr/sbin/useradd -M -N -g mysql -o -r -s /bin/false \
    -c "MySQL Server" -u 27 mysql 

COPY bin/linux_amd64/ndb-agent /

USER mysql

EXPOSE 8080

ENTRYPOINT ["/ndb-agent"]
#ENTRYPOINT ["/bin/bash"]
