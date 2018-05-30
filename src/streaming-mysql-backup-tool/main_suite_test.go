package main_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/nu7hatch/gouuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"gopkg.in/yaml.v2"
	"streaming-mysql-backup-tool/config"
)

var (
	pathToMainBinary string
	configPath       string
)

func TestStreamingBackup(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Streaming Backup Executable Suite")
}

var _ = BeforeSuite(func() {
	var err error
	pathToMainBinary, err = gexec.Build("streaming-mysql-backup-tool")
	Expect(err).ShouldNot(HaveOccurred())

	configPath = createTmpFile("config").Name()
})

func tmpFilePath(filePrefix string) string {
	tempDir, err := ioutil.TempDir(os.TempDir(), "streaming-mysql-backup-tool")
	Expect(err).NotTo(HaveOccurred())

	guid, err := uuid.NewV4()
	Expect(err).NotTo(HaveOccurred())

	//tmpFilePath is in /tmpdir/prefix_guid format
	filename := fmt.Sprintf("%s_%s", filePrefix, guid)
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

func writeConfig(rootConfig config.Config) {
	fileToWrite, err := os.Create(configPath)
	Expect(err).ShouldNot(HaveOccurred())

	marshalled, _ := yaml.Marshal(rootConfig)
	_, err = fileToWrite.Write(marshalled)
	Expect(err).ShouldNot(HaveOccurred())
}

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})
