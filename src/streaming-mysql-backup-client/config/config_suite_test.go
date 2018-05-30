package config_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestMysqlBackupConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Streaming MySQL Backup Config Suite")
}
