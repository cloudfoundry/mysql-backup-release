package xtrabackup_test

import (
	"bytes"
	"database/sql"
	"io"

	"code.cloudfoundry.org/lager/v3/lagertest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/ory/dockertest/v3"

	"github.com/cloudfoundry/streaming-mysql-backup-tool/xtrabackup"
)

var _ = Describe("xtrabackup.Writer", func() {
	var (
		pool       *dockertest.Pool
		container  *dockertest.Resource
		testLogger *lagertest.TestLogger
	)
	BeforeEach(func() {
		testLogger = lagertest.NewTestLogger("xtrabackup")

		var err error
		pool, err = dockertest.NewPool("")
		Expect(err).NotTo(HaveOccurred())

		container, err = pool.RunWithOptions(&dockertest.RunOptions{
			Name:         "mysql." + sessionID,
			Repository:   "percona/percona-server",
			Tag:          "8.0",
			Env:          []string{"MYSQL_ALLOW_EMPTY_PASSWORD=1"},
			ExposedPorts: []string{"3306/tcp"},
			Mounts:       []string{volumeName + ":/var/lib/mysql"},
		})

		db, err := sql.Open("mysql", "root@tcp(localhost:"+container.GetPort("3306/tcp")+")/")
		Expect(err).NotTo(HaveOccurred())
		Expect(pool.Retry(db.Ping)).To(Succeed())
	})

	AfterEach(func() {
		Expect(pool.Purge(container)).To(Succeed())
	})

	It("streams xtrabackup output in the desired format", func() {
		var buf bytes.Buffer
		err := xtrabackup.Writer{
			DefaultsFile: "/etc/my.cnf",
			TmpDir:       "/tmp",
			Logger:       testLogger,
		}.StreamTo("xbstream", &buf)
		Expect(err).NotTo(HaveOccurred())
		Expect(testLogger.Buffer()).To(gbytes.Say(`(?s)"xtrabackup --defaults-file=/etc/my.cnf --backup --stream=xbstream --target-dir=/tmp\\n"`))
	})

	When("specifying an invalid stream format", func() {
		It("returns an error", func() {
			err := xtrabackup.Writer{
				DefaultsFile: "/etc/my.cnf",
				TmpDir:       "/tmp",
				Logger:       testLogger,
			}.StreamTo("invalid", io.Discard)
			Expect(err).To(HaveOccurred())
			Expect(testLogger.Buffer()).To(gbytes.Say(`\[Xtrabackup\] Invalid --stream argument: invalid`))
		})
	})
})
