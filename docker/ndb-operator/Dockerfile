# this is only for a first try - missing USER/security/everything
FROM scratch

COPY bin/linux_amd64/* /usr/sbin/
COPY pkg/helpers/scripts/* /ndb-operator-scripts/

ENTRYPOINT ["ndb-operator"]
