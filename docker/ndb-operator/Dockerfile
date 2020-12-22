# this is only for a first try - missing USER/security/everything
FROM scratch

COPY bin/linux_amd64/ndb-operator /
COPY pkg/helpers/scripts/* /ndb-operator-scripts/

ENTRYPOINT ["/ndb-operator"]
