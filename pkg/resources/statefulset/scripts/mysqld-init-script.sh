#!/bin/bash
# Copyright (c) 2022, 2023, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Docker entrypoint for MySQL Server
# A trimmed down version of https://github.com/mysql/mysql-docker/blob/main/mysql-cluster/8.0/docker-entrypoint.sh
# updated to fit NDB Operator requirements

set -e
set -o nounset

echo "[Entrypoint] MySQL Docker Image"

# Fetch value from server config
# We use mysqld --verbose --help instead of my_print_defaults because the
# latter only show values present in config files, and not server defaults
_get_config() {
	local conf="$1"; shift
	"$@" --verbose --help 2>/dev/null | grep "^$conf" | awk '$1 == "'"$conf"'" { print $2; exit }'
}

# Operator runs the image as root use, but the MySQL Server will be run as mysql user.
install_devnull="install /dev/null -m0600 -omysql -gmysql"
MYSQLD_USER=mysql

# Validate the config passed to the script.
# We redirect stdout to /dev/null so only the error messages are left.
result=0
output=$("$@" --validate-config) || result=$?
if [ ! "$result" = "0" ]; then
  echo >&2 '[Entrypoint] ERROR: Unable to start MySQL. Please check your configuration.'
  echo >&2 "[Entrypoint] $output"
  exit 1
fi

# Extract datadir and socket details from config
DATADIR="$(_get_config 'datadir' "$@")"
SOCKET="$(_get_config 'socket' "$@")"

if [ -f "${DATADIR}/mysql-init-complete" ]; then
  # data directory initialisation is complete
  echo "[Entrypoint] Data directory already exists and is initialized"
  exit 0
fi

# Clear contents of MySQL Data directory that might have
# been left over from a previous failed initialisation
rm -rf "${DATADIR}"/*

echo '[Entrypoint] Initializing database'
"$@" --user=$MYSQLD_USER --initialize-insecure
echo '[Entrypoint] Database initialized'

echo '[Entrypoint] Starting MySQL Server for further initialisations'
"$@" --user=$MYSQLD_USER --daemonize --skip-networking --socket="$SOCKET"

# To avoid using password on commandline, put it in a temporary file.
# The file is only populated when and if the root password is set.
PASSFILE=$(mktemp -u /var/lib/mysql-files/XXXXXXXXXX)
$install_devnull "$PASSFILE"
# Define the client command used throughout the script
# "SET @@SESSION.SQL_LOG_BIN=0;" is required for products like group replication to work properly
mysql=( mysql --defaults-extra-file="$PASSFILE" --protocol=socket -uroot -hlocalhost --socket="$SOCKET" --init-command="SET @@SESSION.SQL_LOG_BIN=0;")

# Wait for MySQL Server to start
for i in {30..0}; do
  if mysqladmin --socket="$SOCKET" ping &>/dev/null; then
    break
  fi
  echo '[Entrypoint] Waiting for server...'
  sleep 1
done
if [ "$i" = 0 ]; then
  echo >&2 '[Entrypoint] Timeout during MySQL init.'
  exit 1
fi

# Install time zones
mysql_tzinfo_to_sql /usr/share/zoneinfo | "${mysql[@]}" mysql

# Create the procedures required for ndbcluster initialisation
"${mysql[@]}" <<-EOSQL
  DELIMITER %%

  # Stored function to test if the ndbcluster engine is ready
  DROP FUNCTION IF EXISTS mysql.IsNdbclusterSetupComplete%%
  CREATE
    DEFINER = 'root'@'localhost'
    FUNCTION mysql.IsNdbclusterSetupComplete()
    RETURNS BOOLEAN
    # execute as root to get ndbcluster binlog thread state
    SQL SECURITY DEFINER
    DETERMINISTIC READS SQL DATA
  BEGIN
    DECLARE isReady BOOLEAN;
    SELECT COUNT(*) = 1 into isReady
    FROM INFORMATION_SCHEMA.PROCESSLIST
    WHERE USER='SYSTEM USER' AND STATE = 'Waiting for event from ndbcluster';
    RETURN isReady;
  END%%

  # Procedure to wait until ndbcluster is set up
  DROP PROCEDURE IF EXISTS mysql.WaitUntilNdbclusterSetupCompletes%%
  CREATE PROCEDURE mysql.WaitUntilNdbclusterSetupCompletes(maxSecondsToWait INT)
    NOT DETERMINISTIC
  BEGIN
    DECLARE isReady BOOLEAN;
    DECLARE counter INT;
    # Check every .2 seconds => Loops 5 * maxSecondsToWait
    SET counter = maxSecondsToWait * 5;
    SELECT mysql.IsNdbclusterSetupComplete() INTO isReady;
    WHILE NOT isReady AND counter > 0 DO
        SET counter = counter - 1;
        DO SLEEP(.2);
        SELECT mysql.IsNdbclusterSetupComplete() INTO isReady;
      END WHILE;
  END%%

  DELIMITER ;
EOSQL

# Get the wait setup value
NDB_WAIT_SETUP="$(_get_config 'ndb-wait-setup' "$@")"

# Wait until ndbcluster is ready
"${mysql[@]}" -e "CALL mysql.WaitUntilNdbclusterSetupCompletes(${NDB_WAIT_SETUP});"

# The NDB Operator user needs to be created only once from the 0th MySQL pod if it doesn't exist already
NDB_OPERATOR_USER="ndb-operator-user"
OPERATOR_USER_CREATE=""
if [[ "$HOSTNAME" == *-mysqld-0 && \
      $("${mysql[@]}" -LNB -e "SELECT COUNT(*) FROM mysql.user WHERE user='${NDB_OPERATOR_USER}' and host='${NDB_OPERATOR_HOST}';") == "0" ]]; then
  OPERATOR_USER_CREATE="CREATE USER '${NDB_OPERATOR_USER}'@'${NDB_OPERATOR_HOST}' IDENTIFIED BY '${NDB_OPERATOR_PASSWORD}'; \
  GRANT ALL ON *.* TO '${NDB_OPERATOR_USER}'@'${NDB_OPERATOR_HOST}' WITH GRANT OPTION;"
fi

# Deduce allowed data node pod hostnames by extracting NdbCluster name from current hostname
NDBCLUSTER_NAME=${HOSTNAME%-mysqld-*}
ALLOWED_DATANODE_HOSTS=${NDBCLUSTER_NAME}-ndbmtd-%.${NDBCLUSTER_NAME}-ndbmtd.${NDB_POD_NAMESPACE}.svc.%

# Create/update the required users
"${mysql[@]}" <<-EOSQL
  # Update the password of the local root user
  ALTER USER 'root'@'localhost' IDENTIFIED BY '${MYSQL_ROOT_PASSWORD}';

  # Create the ndb-operator-user to be used by the NDB Operator
  ${OPERATOR_USER_CREATE}

  # Create another ndb-operator user to be used by the data node pods for ndbinfo lookups
  CREATE USER '${NDB_OPERATOR_USER}'@'${ALLOWED_DATANODE_HOSTS}' IDENTIFIED BY '${NDB_OPERATOR_PASSWORD}';
  GRANT SELECT ON ndbinfo.* TO '${NDB_OPERATOR_USER}'@'${ALLOWED_DATANODE_HOSTS}';

  # Create the healthcheck user
  CREATE USER 'healthchecker'@'localhost' IDENTIFIED BY 'healthcheckpass';
  # Grant privileges for the healthchecker user on mysql.IsNdbclusterSetupComplete function
  GRANT EXECUTE ON FUNCTION mysql.IsNdbclusterSetupComplete TO 'healthchecker'@'localhost';

  FLUSH PRIVILEGES ;
EOSQL

# Put the password into the temporary config file
cat >"$PASSFILE" <<EOF
[client]
password="${MYSQL_ROOT_PASSWORD}"
EOF

# Run other custom scripts in the docker-entrypoint-initdb.d directory
echo
for f in /docker-entrypoint-initdb.d/*; do
  case "$f" in
    *.sql)
      echo "[Entrypoint] running $f"
      "${mysql[@]}" < "$f"
      echo "Command succeeded"
      ;;
    *)     
      echo "[Entrypoint] ignoring $f"
      ;;
  esac
  echo
done

# When using a local socket, mysqladmin shutdown will only complete when the server is actually down
mysqladmin --defaults-extra-file="$PASSFILE" shutdown -uroot --socket="$SOCKET"
rm -f "$PASSFILE"
unset PASSFILE

echo "[Entrypoint] Server shut down"

# Put the healthchecker user and password into a config file
# To be used by health probes to check the status of the MySQL Server
touch "${DATADIR}/healthcheck.cnf"
cat >"${DATADIR}/healthcheck.cnf" <<EOF
[client]
user=healthchecker
socket=${SOCKET}
password=healthcheckpass
EOF
touch "${DATADIR}/mysql-init-complete"

echo
echo '[Entrypoint] MySQL init process done. Ready for start up.'
echo
