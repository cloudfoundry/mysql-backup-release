package integration_test

import (
	"log"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"github.com/cloudfoundry/specs/docker"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

const (
	DockerImage = "dedicatedmysql/percona-server"
)

var (
	dockerNetwork                     string
	streamingMySQLBackupToolBinPath   string
	streamingMySQLBackupClientBinPath string
	fixturesPath                      string
)

var _ = BeforeSuite(func() {

	Expect(mysql.SetLogger(
		log.New(GinkgoWriter, "[mysql-backup-release:integration:mysql-connector] ", log.Ldate|log.Ltime|log.Lshortfile),
	)).To(Succeed())

	log.SetOutput(GinkgoWriter)

	var err error
	var cwd string

	Expect(docker.PullImage(DockerImage + ":latest")).Error().NotTo(HaveOccurred())

	dockerNetwork = "mysql-net." + uuid.New().String()
	Expect(docker.CreateNetwork(dockerNetwork)).To(Succeed())

	// Default tmpdir on OS X cannot be mapped into a docker container, so use /tmp instead
	Expect(os.Setenv("TMPDIR", "/tmp")).To(Succeed())

	fixturesPath, err = filepath.Abs("fixtures")
	Expect(err).NotTo(HaveOccurred())

	cwd, err = os.Getwd()
	Expect(err).NotTo(HaveOccurred())

	Expect(os.Chdir(path.Join(cwd, "../../streaming-mysql-backup-tool"))).To(Succeed())
	streamingMySQLBackupToolBinPath, err = gexec.BuildWithEnvironment(
		"github.com/cloudfoundry/streaming-mysql-backup-tool",
		[]string{
			"GOOS=linux",
			"GOARCH=amd64",
			"CGO_ENABLED=0",
		},
	)
	Expect(err).NotTo(HaveOccurred())

	Expect(os.Chdir(path.Join(cwd, "../../streaming-mysql-backup-client"))).To(Succeed())
	streamingMySQLBackupClientBinPath, err = gexec.BuildWithEnvironment(
		"github.com/cloudfoundry/streaming-mysql-backup-client",
		[]string{
			"GOOS=linux",
			"GOARCH=amd64",
			"CGO_ENABLED=0",
		},
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.Chdir(cwd)).To(Succeed())
})

var _ = AfterSuite(func() {
	Expect(docker.RemoveNetwork(dockerNetwork)).To(Succeed())
})
