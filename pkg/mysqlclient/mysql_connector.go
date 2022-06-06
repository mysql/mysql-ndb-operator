// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mysqlclient

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// Connect creates a valid db connection to the MySQL server running in the given Ip address
// and returns the connection on success.
func Connect(clientset kubernetes.Interface, nc *v1alpha1.NdbCluster, dbname string, mysqldIp string) *sql.DB {

	if nc.GetMySQLServerNodeCount() < 0 {
		klog.Fatalf("No MySQL Servers configured for NdbCluster %q", nc.Name)
		return nil
	}
	port := 3306

	user := "ndb-operator-user"
	password := "Operator@123"
	dataSource := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=10s", user, password, mysqldIp, port, dbname)
	db, err := sql.Open(dbname, dataSource)
	if err != nil {
		klog.Infof("Error opening connection to MySQL server: %s", err.Error())
		return nil
	}

	// Verify the DB is connected
	if db == nil || db.Ping() != nil {
		klog.Infof("Error connecting to the MySQL server in %s", mysqldIp)
		return nil
	}

	// Recommended settings
	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	return db
}
