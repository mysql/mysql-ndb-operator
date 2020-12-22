#!/bin/bash

# Copyright (c) 2020, Oracle and/or its affiliates.
#
# Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

# Script to do the additional Ndb Operator specific initializations

function fatal_error() {
  local error=$1
  echo >&2 "[Ndb Operator Initializer] ERROR : ${error}"
  exit 1
}

function info() {
  local message=$1
  echo "[Ndb Operator Initializer] INFO : ${message}"
}

"${mysql[@]}" <<-EOSQL
  # Procedures to initialize MySQL Server from within a container
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

  # Procedure to wait for expected number of replicas to complete basic setup
  DROP PROCEDURE IF EXISTS mysql.WaitForExpectedReplicas%%
  CREATE PROCEDURE mysql.WaitForExpectedReplicas(expectedReplicas INT, maxSecondsToWait INT)
  NOT DETERMINISTIC
  BEGIN
    DECLARE done BOOLEAN;
    DECLARE counter INT;
    # Check every .2 seconds => Loops 5 * maxSecondsToWait
    SET counter = maxSecondsToWait * 5;
    # Do not expect the leader entry yet
    SET expectedReplicas = expectedReplicas - 1;
    SELECT count(*) = expectedReplicas INTO done FROM mysql.ndb_operator_init_status;
    WHILE NOT done AND counter > 0 DO
      SET counter = counter - 1;
      DO SLEEP(.2);
      SELECT count(*) = expectedReplicas INTO done FROM mysql.ndb_operator_init_status;
    END WHILE;
  END%%

  # Procedure to wait for the initialisation leader to complete setup
  DROP PROCEDURE IF EXISTS mysql.WaitForLeaderInitDone%%
  CREATE PROCEDURE mysql.WaitForLeaderInitDone(replicaName CHAR(255), maxSecondsToWait INT)
  NOT DETERMINISTIC
  WaitForInit:BEGIN
    DECLARE done BOOLEAN;
    DECLARE counter INT;
    # Check every .2 seconds => Loops 5 * maxSecondsToWait
    SET counter = maxSecondsToWait * 5;

    # Setup is complete if leader in the init table says done.
    SELECT count(*) = 1 INTO done FROM mysql.ndb_operator_init_status WHERE init_done = true;
    IF done THEN LEAVE WaitForInit;
    END IF;
    # Leader is doing setup => its waiting for all replicas to show up and wait
    # Add an entry to the table to inform leader we are waiting as well
    INSERT INTO mysql.ndb_operator_init_status (replica_name) VALUES (replicaName);

    # Wait for one entry in the table to complete init
    WHILE NOT done AND counter > 0 DO
      SET counter = counter - 1;
      DO SLEEP(.2);
      SELECT count(*) = 1 INTO done FROM mysql.ndb_operator_init_status WHERE init_done = true;
    END WHILE;
  END%%

  # Procedure to create a new root user at given host
  DROP PROCEDURE IF EXISTS mysql.CreateRootAtHost%%
  CREATE PROCEDURE mysql.CreateRootAtHost(root_host CHAR(255), root_pass CHAR(64))
  DETERMINISTIC MODIFIES SQL DATA
  BEGIN
    # Create user 'root'@'root_host'
    # It will be distributed to all connected MySQL Servers during GRANT ALL
    SET @root_user := CONCAT('''root''@''', root_host, '''');
    SET @create_stmt := CONCAT('CREATE USER ', @root_user, ' IDENTIFIED BY ''', root_pass, ''';');
    PREPARE stmt FROM @create_stmt;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;

    # Give all grants
    SET @grant_stmt1 := CONCAT('GRANT ALL ON *.* TO ', @root_user,' WITH GRANT OPTION');
    PREPARE stmt FROM @grant_stmt1;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;

    SET @grant_stmt2 := CONCAT('GRANT PROXY ON ''''@'''' TO ', @root_user,' WITH GRANT OPTION');
    PREPARE stmt FROM @grant_stmt2;
    EXECUTE stmt;
    DEALLOCATE PREPARE stmt;

    FLUSH PRIVILEGES;

    SELECT CONCAT('User ''root''@''', root_host,''' has been created') AS Status;
  END%%

  DELIMITER ;
EOSQL

# Get the wait setup value
NDB_WAIT_SETUP="$(_get_config 'ndb-wait-setup' "$@")"

# Wait until ndbcluster is ready
"${mysql[@]}" -e "CALL mysql.WaitUntilNdbclusterSetupCompletes(${NDB_WAIT_SETUP});"

# The further initialisations done by this script needs to be run only
# from one MySQL Server as they get distributed to other Servers via NDB.
# To prevent all Servers from running this script, we elect a leader who
# continues with the initialisation. Other Servers wait for the leader to
# complete. To elect the leader, we use the result of the following DDL.
# Since mysql.ndb_operator_init_status is an NDB table, only the first
# MySQL Server can successfully execute it. Others that follow will get
# a table exists error. The one that succeeded is elected leader.
CREATE_INIT_TABLE="CREATE TABLE mysql.ndb_operator_init_status (
                     replica_name VARCHAR(255) PRIMARY KEY,
                     init_done BOOLEAN DEFAULT false
                   ) ENGINE NDB;"

# TODO: Handle failure from leader
if "${mysql[@]}" -e "${CREATE_INIT_TABLE}" ; then
  # Verify that the expected replicas and root host are passed via environment
  if [[ -z "${MYSQL_CLUSTER_ROOT_HOST}" ]] || [[ -z "${MYSQL_CLUSTER_EXPECTED_REPLICAS}" ]]; then
    fatal_error "Please set MYSQL_CLUSTER_ROOT_HOST and MYSQL_CLUSTER_EXPECTED_REPLICAS"
  fi
  # The init table was just now created by this Server
  # This Server is now the leader for initialisations
  # Continue with further initialisations as follows
  # 1. Wait for all other replicas to reach this point
  #   - This is required as we don't want to accidentally
  #     distribute users to a Server when it is mid way through
  #     binlog setup. This is possible as the ACL statements are
  #     not protected by GSL and will cause issues. (Bug#31680765)
  #     TODO: Revisit this code after the bug gets fixed. The leader
  #           doesn't have to wait if it can execute ACL safely.
  # 2. CREATE and GRANT ALL to the user root at requested host
  # 3. Inform the other server that init is complete by
  #    updating mysql.ndb_operator_init_status
  # 4. Cleanup obsolete rows in mysql.ndb_operator_init_status
  INIT_SQL="CALL mysql.WaitForExpectedReplicas(${MYSQL_CLUSTER_EXPECTED_REPLICAS}, ${NDB_WAIT_SETUP});
            CALL mysql.CreateRootAtHost('${MYSQL_CLUSTER_ROOT_HOST}', '${MYSQL_ROOT_PASSWORD}');
            INSERT INTO mysql.ndb_operator_init_status VALUES ('${HOSTNAME}', true);
            DELETE FROM mysql.ndb_operator_init_status WHERE init_done = false;"
else
  # Table already exists => either setup is complete
  # or some other Server has been elected leader.
  # In any case, we wait until the leader is done.
  # Wait for ${NDB_WAIT_SETUP} + 10 secs for all the additional inits done by leader
  NDB_WAIT_SETUP_TOTAL=$((NDB_WAIT_SETUP + 10))
  INIT_SQL="CALL mysql.WaitForLeaderInitDone('${HOSTNAME}', ${NDB_WAIT_SETUP_TOTAL});"
fi

"${mysql[@]}" <<-EOSQL
  ${INIT_SQL}
  # Grant privileges for the healthchecker user on mysql.IsNdbclusterSetupComplete function
  GRANT EXECUTE ON FUNCTION mysql.IsNdbclusterSetupComplete TO 'healthchecker'@'localhost';
  FLUSH PRIVILEGES;
EOSQL

info "Successfully initialized MySQL Cluster Server!"
