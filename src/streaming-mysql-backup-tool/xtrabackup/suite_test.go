package xtrabackup_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	_ "github.com/go-sql-driver/mysql"
)

var (
	sessionID  string
	volumeName string
)

func TestXtrabackup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Xtrabackup Suite")
}

var _ = BeforeSuite(func() {
	sessionID = uuid.New().String()

	volumeName = "xtrabackup-integration-test-data." + sessionID
	scriptsDir, err := filepath.Abs("../bin")
	Expect(err).NotTo(HaveOccurred())

	Expect(os.Setenv("PATH", scriptsDir+":"+os.Getenv("PATH"))).To(Succeed())
	Expect(os.Setenv("MYSQL_VOLUME", volumeName)).To(Succeed())
})

var _ = AfterSuite(func() {
	cmd := exec.Command("docker", "volume", "remove", "--force", volumeName)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	Expect(cmd.Run()).To(Succeed())
})
