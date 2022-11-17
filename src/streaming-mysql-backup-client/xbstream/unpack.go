package xbstream

import (
	"fmt"
	"io"
	"os/exec"
)

type Unpacker struct {
	destDir string
}

func NewUnpacker(destinationDir string) Unpacker {
	return Unpacker{
		destDir: destinationDir,
	}
}

func (us Unpacker) WriteStream(reader io.Reader) error {
	cmd := exec.Command("xbstream", "-x", "-C", us.destDir)
	cmd.Stdin = reader
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("xbstream failed: %v - %v", err, string(output))
	}

	return nil
}
