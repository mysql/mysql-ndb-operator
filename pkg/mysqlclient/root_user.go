// Copyright (c) 2022, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mysqlclient

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	appsv1 "k8s.io/api/apps/v1"
	klog "k8s.io/klog/v2"
)

// rootUserExists returns true if the root user with given rootHost exists
func rootUserExists(db *sql.DB, rootHost string) (bool, error) {
	var count int
	query := fmt.Sprintf("select count(*) from user where host='%s' and user='root'", rootHost)
	err := db.QueryRow(query).Scan(&count)
	if err != nil {
		klog.Infof("Error executing %s: %s", query, err.Error())
		return false, err
	}

	return count != 0, nil
}

// CreateRootUserIfNotExist creates root user if it does not exist already
func CreateRootUserIfNotExist(mysqldSfset *appsv1.StatefulSet, rootHost, rootPassword string, ndbOperatorPassword string) error {

	db, err := ConnectToStatefulSet(mysqldSfset, DbMySQL, ndbOperatorPassword)
	if err != nil {
		return err
	}

	if exists, err := rootUserExists(db, rootHost); err != nil {
		return err
	} else if exists {
		return nil
	}

	// The root user with given host does not exist.
	// So, create user in database
	klog.Infof("Creating the root user with host = %s", rootHost)
	query := fmt.Sprintf("create user 'root'@'%s' identified by '%s'", rootHost, rootPassword)
	_, err = db.Exec(query)
	if err != nil {
		klog.Infof("Error executing %s: %s", query, err.Error())
		return err
	}

	query = fmt.Sprintf("grant all on *.* to 'root'@'%s' with grant option", rootHost)
	_, err = db.Exec(query)
	if err != nil {
		klog.Infof("Error executing %s: %s", query, err.Error())
		return err
	}

	_, err = db.Exec("flush privileges")
	if err != nil {
		klog.Infof("Error executing flush privileges: %s", err.Error())
		return err
	}

	return nil
}

// DeleteRootUserIfExists deletes the root user from the database.
func DeleteRootUserIfExists(mysqldSfset *appsv1.StatefulSet, rootHost string, ndbOperatorPassword string) error {

	db, err := ConnectToStatefulSet(mysqldSfset, DbMySQL, ndbOperatorPassword)
	if err != nil {
		return err
	}

	if exists, err := rootUserExists(db, rootHost); err != nil {
		return err
	} else if !exists {
		// No need to delete
		return nil
	}

	klog.Infof("Deleting the root user with host = %s", rootHost)
	query := fmt.Sprintf("drop user 'root'@'%s'", rootHost)
	_, err = db.Exec(query)
	if err != nil {
		klog.Infof("Error executing %s: %s", query, err.Error())
		return err
	}

	return nil
}

// UpdateRootUser updates the host name of an existing root user in the database.
func UpdateRootUser(mysqldSfset *appsv1.StatefulSet, oldRootHost, newRootHost string, ndbOperatorPassword string) error {
	db, err := ConnectToStatefulSet(mysqldSfset, DbMySQL, ndbOperatorPassword)
	if err != nil {
		return err
	}

	klog.Infof("Updating the host name of root user from %s to %s", oldRootHost, newRootHost)
	query := fmt.Sprintf("rename user 'root'@'%s' to 'root'@'%s'", oldRootHost, newRootHost)
	_, err = db.Exec(query)
	if err != nil {
		klog.Infof("Error executing %s: %s", query, err.Error())
		return err
	}

	_, err = db.Exec("flush privileges")
	if err != nil {
		klog.Infof("Error executing flush privileges: %s", err.Error())
		return err
	}
	return nil
}
