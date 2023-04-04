package cryptkeeper_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMysqlBackupInitiator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CryptKeeper tests")
}

var _ = BeforeSuite(func() {
})

var _ = AfterSuite(func() {
})
