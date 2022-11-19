package prepare

import (
	"os/exec"
)

type BackupPreparer struct {
}

func DefaultBackupPreparer() *BackupPreparer {
	return &BackupPreparer{}
}

func (*BackupPreparer) Command(backupDir string) *exec.Cmd {
	// TODO: remove hardcoded path
	return exec.Command("/var/vcap/packages/percona-xtrabackup-8.0/bin/xtrabackup", "--prepare", "--target-dir", backupDir)
}
