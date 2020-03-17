package prepare_test

import (
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"streaming-mysql-backup-client/prepare"
)

var _ = Describe("Prepare Command", func() {
	It("Uses innobackupex", func() {
		backupPrepare := prepare.DefaultBackupPreparer()

		cmd := backupPrepare.Command("path/to/backup")

		Expect(filepath.Base(cmd.Path)).To(Equal("xtrabackup"))
		Expect(cmd.Args[1:]).To(Equal([]string{"--prepare", "--target-dir", "path/to/backup"}))
	})
})
