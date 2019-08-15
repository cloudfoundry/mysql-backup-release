package integration_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/fsouza/go-dockerclient"
	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/pivotal/mysql-test-utils/dockertest"
	"github.com/pivotal/mysql-test-utils/testhelpers"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

const (
	DockerImage = "dedicatedmysql/percona-server"
)

var (
	dockerClient                      *docker.Client
	dockerNetwork                     *docker.Network
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

	dockerClient, err = docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())

	Expect(dockertest.PullImage(dockerClient, DockerImage)).To(Succeed())

	dockerNetwork, err = dockertest.CreateNetwork(dockerClient, "mysql-net."+uuid.New().String())
	Expect(err).NotTo(HaveOccurred())

	// Default tmpdir on OS X cannot be mapped into a docker container, so use /tmp instead
	Expect(os.Setenv("TMPDIR", "/tmp")).To(Succeed())

	streamingMySQLBackupToolBinPath, err = gexec.BuildWithEnvironment(
		"streaming-mysql-backup-tool",
		[]string{
			"GOOS=linux",
			"GOARCH=amd64",
			"CGO_ENABLED=0",
		},
	)
	Expect(err).NotTo(HaveOccurred())

	streamingMySQLBackupClientBinPath, err = gexec.BuildWithEnvironment(
		"streaming-mysql-backup-client",
		[]string{
			"GOOS=linux",
			"GOARCH=amd64",
			"CGO_ENABLED=0",
		},
	)
	Expect(err).NotTo(HaveOccurred())

	fixturesPath, err = filepath.Abs("fixtures")
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if dockerNetwork != nil {
		Expect(dockertest.RemoveNetwork(dockerClient, dockerNetwork)).To(Succeed())
	}
})

var _ = JustAfterEach(func() {
	if CurrentGinkgoTestDescription().Failed {
		fmt.Fprint(GinkgoWriter, testhelpers.TestFailureMessage)
	}
})
