// Copyright (c) 2021, 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/mysql/ndb-operator/e2e-tests/utils/secret"
	"github.com/mysql/ndb-operator/e2e-tests/utils/service"
	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
)

// Connect extracts the ip address of the MySQL Load balancer service and creates a connection to it
func Connect(clientset kubernetes.Interface, nc *v1alpha1.NdbCluster, dbname string) *sql.DB {

	gomega.Expect(nc.GetMySQLServerNodeCount()).NotTo(
		gomega.BeZero(), fmt.Sprintf("No MySQL Servers configured for NdbCluster %q", nc.Name))

	ginkgo.By("connecting to the MySQL Load balancer")
	serviceName := nc.GetServiceName("mysqld") + "-ext"
	host, port := service.GetServiceAddressAndPort(clientset, nc.Namespace, serviceName)
	user := "root"
	password := secret.GetMySQLRootPassword(context.TODO(), clientset, nc)
	dataSource := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=10s", user, password, host, port, dbname)
	db, err := sql.Open("mysql", dataSource)
	gomega.Expect(err).Should(gomega.Succeed())
	// Use retry to connect to the server as
	// the connection initially fails sometimes.
	retry := 20
	retryInterval := 50 * time.Microsecond
	for db.Ping() != nil && retry > 0 {
		time.Sleep(retryInterval)
		retry--
	}
	// Verify the DB is connected
	gomega.Expect(db.Ping()).Should(gomega.Succeed())

	// Recommended settings
	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	return db
}
