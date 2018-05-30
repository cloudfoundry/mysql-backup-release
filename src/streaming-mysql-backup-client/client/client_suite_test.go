package client_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestMysqlBackupInitiator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Streaming MySQL Backup Client Suite")
}
