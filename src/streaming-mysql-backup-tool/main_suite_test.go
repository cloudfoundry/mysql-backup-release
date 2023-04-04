package main_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var (
	pathToMainBinary string
	configPath       string
	sessionID        string
	volumeName       string
)

func TestStreamingBackup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Streaming Backup Executable Suite")
}

var _ = BeforeSuite(func() {
	SetDefaultEventuallyTimeout(10 * time.Second)

	sessionID = uuid.New().String()

	volumeName = "xtrabackup-integration-test-data." + sessionID
	scriptsDir, err := filepath.Abs("bin")
	Expect(err).NotTo(HaveOccurred())
	Expect(os.Setenv("PATH", scriptsDir+":"+os.Getenv("PATH"))).To(Succeed())
	Expect(os.Setenv("MYSQL_VOLUME", volumeName)).To(Succeed())

	pathToMainBinary, err = gexec.Build("github.com/cloudfoundry/streaming-mysql-backup-tool")
	Expect(err).ShouldNot(HaveOccurred())

	configPath = createTmpFile("config").Name()

	Expect(docker("pull", "--quiet", "percona/percona-server:8.0")).To(Succeed())
	Expect(docker("pull", "--quiet", "percona/percona-xtrabackup:8.0")).To(Succeed())
})

func docker(args ...string) error {
	cmd := exec.Command("docker", args...)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	return cmd.Run()
}

func tmpFilePath(filePrefix string) string {
	tempDir, err := ioutil.TempDir(os.TempDir(), "streaming-mysql-backup-tool")
	Expect(err).NotTo(HaveOccurred())

	//tmpFilePath is in /tmpdir/prefix_guid format
	filename := fmt.Sprintf("%s_%s", filePrefix, uuid.New().String())
	tmpFilePath := filepath.Join(tempDir, filename)

	return tmpFilePath
}

func createTmpFile(filePrefix string) *os.File {
	tempDir, err := ioutil.TempDir(os.TempDir(), "streaming-mysql-backup-tool")
	Expect(err).NotTo(HaveOccurred())

	tmpFile, err := ioutil.TempFile(tempDir, filePrefix)
	Expect(err).NotTo(HaveOccurred())

	return tmpFile
}

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})
