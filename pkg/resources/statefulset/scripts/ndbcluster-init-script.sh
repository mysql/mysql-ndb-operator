#!/bin/bash

# Copyright (c) 2020, 2022, Oracle and/or its affiliates.
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

  # Procedure to create a new root user at given host
  DROP PROCEDURE IF EXISTS mysql.CreateUserForOperator %%
  CREATE PROCEDURE mysql.CreateUserForOperator (operator_host CHAR(255))
  DETERMINISTIC MODIFIES SQL DATA
  BEGIN
    DECLARE userexists BOOLEAN;
    SELECT COUNT(*) = 1 INTO userexists FROM mysql.user WHERE user='ndb-operator-user';
    IF userexists THEN
      SELECT CONCAT('User ''ndb-operator-user'' exists already') AS Status;
    ELSE
      # Create user for operator 'ndb-operator-user'@'operator_host'
      # It will be distributed to all connected MySQL Servers during GRANT ALL
      SET @operator_root_user := CONCAT('''ndb-operator-user''@''', operator_host, '''');
      SET @create_stmt := CONCAT('CREATE USER ', @operator_root_user, ' IDENTIFIED BY ''', 'Operator@123', ''';');
      PREPARE stmt FROM @create_stmt;
      EXECUTE stmt;
      DEALLOCATE PREPARE stmt;

      # Give all grants
      SET @grant_stmt := CONCAT('GRANT ALL ON *.* TO ', @operator_root_user,' WITH GRANT OPTION');
      PREPARE stmt FROM @grant_stmt;
      EXECUTE stmt;
      DEALLOCATE PREPARE stmt;
      FLUSH PRIVILEGES;

      SELECT CONCAT('User ''ndb-operator-user''@''', operator_host, ''' has been created') AS Status;
    END IF;
  END%%

  DELIMITER ;
EOSQL

# Get the wait setup value
NDB_WAIT_SETUP="$(_get_config 'ndb-wait-setup' "$@")"

# Wait until ndbcluster is ready
"${mysql[@]}" -e "CALL mysql.WaitUntilNdbclusterSetupCompletes(${NDB_WAIT_SETUP});"


INIT_SQL="CALL mysql.CreateUserForOperator ('${NDB_OPERATOR_ROOT_HOST}');"

NDBCLUSTER_NAME=${HOSTNAME%-mysqld-*}
ALLOWED_DATANODE_HOSTS=${NDBCLUSTER_NAME}-ndbmtd-%.${NDBCLUSTER_NAME}-ndbmtd.${NDB_POD_NAMESPACE}.svc.%

"${mysql[@]}" <<-EOSQL
  ${INIT_SQL}
  # Grant privileges for the healthchecker user on mysql.IsNdbclusterSetupComplete function
  GRANT EXECUTE ON FUNCTION mysql.IsNdbclusterSetupComplete TO 'healthchecker'@'localhost';
  # Create another user to be used by the data node pods for ndbinfo lookups
  CREATE USER IF NOT EXISTS 'ndb-operator-user'@'${ALLOWED_DATANODE_HOSTS}' IDENTIFIED BY 'Operator@123';
  GRANT SELECT ON ndbinfo.* TO 'ndb-operator-user'@'${ALLOWED_DATANODE_HOSTS}';
  FLUSH PRIVILEGES;
EOSQL

info "Successfully initialized MySQL Cluster Server!"
