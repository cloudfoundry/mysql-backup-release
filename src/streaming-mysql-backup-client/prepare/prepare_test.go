package prepare_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"streaming-mysql-backup-client/prepare"
)

var _ = Describe("Prepare Command", func() {
	It("Uses innobackupex", func() {
		backupPrepare := prepare.DefaultBackupPreparer()

		cmd := backupPrepare.Command("path/to/backup")

		Expect(cmd.Path).To(Equal("/var/vcap/packages/xtrabackup/bin/xtrabackup"))
		Expect(cmd.Args[1:]).To(Equal([]string{"--prepare", "--target-dir", "path/to/backup"}))
	})
})
