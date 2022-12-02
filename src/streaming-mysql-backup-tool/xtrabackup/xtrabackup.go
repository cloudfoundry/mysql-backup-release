package xtrabackup

import (
	"errors"
	"io"
	"os/exec"

	"code.cloudfoundry.org/lager"

	"github.com/cloudfoundry/streaming-mysql-backup-tool/api"
)

type LoggerWriter struct {
	logger lager.Logger
}

func (lw *LoggerWriter) Write(p []byte) (int, error) {
	lw.logger.Error("xtrabackup", errors.New(string(p[:])))
	return len(p), nil
}

type Writer struct {
	DefaultsFile string
	TmpDir       string
	Logger       lager.Logger
}

func (x Writer) StreamTo(format string, w io.Writer) error {
	cmd := exec.Command("xtrabackup", "--defaults-file="+x.DefaultsFile, "--backup", "--stream="+format, "--target-dir="+x.TmpDir)
	cmd.Stdout = w
	cmd.Stderr = &LoggerWriter{logger: x.Logger}
	return cmd.Run()
}

var _ api.BackupWriter = &Writer{}
