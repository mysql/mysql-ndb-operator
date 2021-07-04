// Copyright (c) 2021, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mysql

import (
	"database/sql"
	"fmt"
	"github.com/mysql/ndb-operator/e2e-tests/utils/service"
	"github.com/onsi/ginkgo"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Connect extracts the ip address of the MySQL Load balancer service and creates a connection to it
func Connect(clientset kubernetes.Interface, namespace string, ndbName string, dbname string) *sql.DB {

	ginkgo.By("connecting to the MySQL Load balancer")
	serviceName := ndbName + "-mysqld-ext"
	host, port := service.GetServiceAddressAndPort(clientset, namespace, serviceName)
	user := "root"
	// TODO: Auto detect password from Ndb object
	password := "ndbpass"
	dataSource := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", user, password, host, port, dbname)
	db, err := sql.Open("mysql", dataSource)
	framework.ExpectNoError(err)

	// Recommended settings
	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	return db
}
