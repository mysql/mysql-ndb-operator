package mysql

import (
	"database/sql"
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
	host := service.GetExternalIP(clientset, namespace, serviceName)
	user := "root"
	// TODO: Auto detect password from Ndb object
	password := "ndbpass"
	db, err := sql.Open("mysql", user+":"+password+"@tcp("+host+")/"+dbname)
	framework.ExpectNoError(err)

	// Recommended settings
	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

	return db
}
