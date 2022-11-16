package e2e_tests

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/proxy"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MySQL Backup E2E Tests")
}

var (
	proxyDialer proxy.DialContextFunc
	backupPath string
	myLoginCnfFilePath string
	mysqlDatdir string
)

var _ = BeforeSuite(func() {
	var missingEnvs []string
	for _, v := range []string{
		"BOSH_ENVIRONMENT",
		"BOSH_CA_CERT",
		"BOSH_CLIENT",
		"BOSH_CLIENT_SECRET",
		"CREDHUB_SERVER",
		"CREDHUB_CLIENT",
		"CREDHUB_SECRET",
	} {
		if os.Getenv(v) == "" {
			missingEnvs = append(missingEnvs, v)
		}
	}
	Expect(missingEnvs).To(BeEmpty(), "Missing environment variables: %s", strings.Join(missingEnvs, ", "))

	if proxySpec := os.Getenv("BOSH_ALL_PROXY"); proxySpec != "" {
		var err error
		proxyDialer, err = proxy.NewDialerViaSSH(context.Background(), proxySpec)
		Expect(err).NotTo(HaveOccurred())

		mysql.RegisterDialContext("tcp", func(ctx context.Context, addr string) (net.Conn, error) {
			return proxyDialer(ctx, "tcp", addr)
		})
	}

	backupPath = "/var/vcap/store/mysql-backups/"
	if mysqlEngine := os.Getenv("MYSQL_ENGINE"); strings.Contains(mysqlEngine,"dedicated-mysql") {
		mysqlDatdir = "/var/vcap/store/mysql/data"
		myLoginCnfFilePath = "/var/vcap/jobs/mysql/config/mylogin.cnf"
	} else {
		mysqlDatdir = "/var/vcap/store/pxc-mysql"
		myLoginCnfFilePath = "/var/vcap/jobs/pxc-mysql/config/mylogin.cnf"
	}
})
