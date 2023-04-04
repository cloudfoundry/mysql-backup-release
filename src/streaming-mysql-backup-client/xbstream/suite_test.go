package xbstream_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestXbstream(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Xbstream Suite")
}

var _ = BeforeSuite(func() {
	scriptDir, err := filepath.Abs("scripts")
	Expect(err).NotTo(HaveOccurred())

	Expect(os.Setenv("PATH", scriptDir+":"+os.Getenv("PATH"))).To(Succeed())
})
