package prepare_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestBackupPreparer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Backup Preparer Suite")
}
