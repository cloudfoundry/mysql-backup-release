package tarpit

import (
	"fmt"
	"io"
	"os/exec"
)

type UntarStreamer struct {
	tarclient *TarClient
	destDir   string
}

func NewUntarStreamer(destinationDir string) UntarStreamer {
	return UntarStreamer{
		tarclient: NewSystemTarClient(),
		destDir:   destinationDir,
	}
}

func (us UntarStreamer) WriteStream(reader io.Reader) error {
	cmd := exec.Command(us.tarclient.TarCommand, "-x", "-C", us.destDir)
	cmd.Stdin = reader
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v - %v", err, string(output))
	}

	return nil
}
