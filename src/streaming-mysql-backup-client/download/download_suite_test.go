package download_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestMysqlBackupInitiator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Streaming MySQL Backup Downlod Backup Suite")
}
