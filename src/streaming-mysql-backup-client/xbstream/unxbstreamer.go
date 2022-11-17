package xbstream

import (
	"fmt"
	"io"
	"os/exec"
)

type UnXbStreamer struct {
	destDir   string
}

func NewUnXbStreamer(destinationDir string) UnXbStreamer {
	return UnXbStreamer{
		destDir:   destinationDir,
	}
}

func (us UnXbStreamer) WriteStream(reader io.Reader) error {
	cmd := exec.Command("/var/vcap/packages/percona-xtrabackup-8.0/bin/xbstream", "-x", "-C", us.destDir)
	cmd.Stdin = reader
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v - %v", err, string(output))
	}

	return nil
}
