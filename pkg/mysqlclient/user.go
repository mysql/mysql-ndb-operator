// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mysqlclient

import (
	"errors"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// CreateUserIfNotExist creates root user if not exist in the database.
func CreateUserIfNotExist(clientset kubernetes.Interface, nc *v1alpha1.NdbCluster, password string, host string, mysqldIp string) error {
	var count int
	var query string
	db := Connect(clientset, nc, "mysql", mysqldIp)
	if db == nil {
		return errors.New("none of the MySQL server in the cluster is reachable")
	}
	query = fmt.Sprintf("select count(*) from user where host='%s' and user='root'", host)
	err := db.QueryRow(query).Scan(&count)
	if err != nil {
		klog.Infof("Error executing %s: %s", query, err.Error())
		return err
	}
	if count == 0 {
		// The root user with given host names does not exist. So, create user in database
		klog.Infof("Creating the root user with host = %s", host)
		query = fmt.Sprintf("create user 'root'@'%s' identified by '%s'", host, password)
		_, err := db.Exec(query)
		if err != nil {
			klog.Infof("Error executing %s: %s", query, err.Error())
			return err
		}

		query = fmt.Sprintf("grant all on *.* to 'root'@'%s' with grant option", host)
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
	}
	return nil
}

// DeleteUserIfItExists deletes the root user from the database.
func DeleteUserIfItExists(clientset kubernetes.Interface, nc *v1alpha1.NdbCluster, host string, mysqldIp string) error {
	var count int
	var query string
	db := Connect(clientset, nc, "mysql", mysqldIp)
	if db == nil {
		return errors.New("none of the MySQL server in the cluster is reachable")
	}
	query = fmt.Sprintf("select count(*) from user where host='%s' and user='root'", host)
	err := db.QueryRow(query).Scan(&count)
	if err != nil {
		klog.Infof("Error executing %s: %s", query, err.Error())
		return err
	}
	if count != 0 {
		klog.Infof("Deleting the root user with host = %s", host)
		query = fmt.Sprintf("drop user 'root'@'%s'", host)
		_, err := db.Exec(query)
		if err != nil {
			klog.Infof("Error executing %s: %s", query, err.Error())
			return err
		}
	}
	return nil
}

// UpdateUser updates the host name of an exisitng root user in the database.
func UpdateUser(clientset kubernetes.Interface, nc *v1alpha1.NdbCluster, newhost string, oldhost string, mysqldIp string) error {
	var query string
	db := Connect(clientset, nc, "mysql", mysqldIp)
	if db == nil {
		return errors.New("none of the MySQL server in the cluster is reachable")
	}

	klog.Infof("Updating the host name of root user from %s to %s", oldhost, newhost)
	query = fmt.Sprintf("rename user 'root'@'%s' to 'root'@'%s'", oldhost, newhost)
	_, err := db.Exec(query)
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
