FROM oraclelinux:8-slim

COPY bin/linux_amd64/* /usr/sbin/
COPY pkg/helpers/scripts/* /ndb-operator-scripts/

ENTRYPOINT ["ndb-operator"]
