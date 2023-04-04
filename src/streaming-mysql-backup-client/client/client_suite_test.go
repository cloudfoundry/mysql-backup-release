package client_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMysqlBackupInitiator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Streaming MySQL Backup Client Suite")
}

var _ = BeforeSuite(func() {
	scriptDir, err := filepath.Abs("../xbstream/scripts")
	Expect(err).NotTo(HaveOccurred())
	Expect(os.Setenv("PATH", scriptDir+":"+os.Getenv("PATH"))).To(Succeed())
})
