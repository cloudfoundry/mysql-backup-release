package tarpit

import (
	"fmt"
	"os/exec"
	"runtime"
)

type TarClient struct {
	TarCommand string
}

func NewOSXTarClient() *TarClient {
	return NewTarClient("gtar")
}

func NewGnuTarClient() *TarClient {
	return NewTarClient("tar")
}

func NewTarClient(tarCommand string) *TarClient {
	return &TarClient{
		TarCommand: tarCommand,
	}
}

func NewSystemTarClient() *TarClient {
	switch runtime.GOOS {
	case "darwin":
		return NewOSXTarClient()
	case "linux":
		return NewGnuTarClient()
	default:
		panic("unrecognized runtime/os")
	}
}

func (this TarClient) Untar(tarfileName, outputDir string) *exec.Cmd {
	return exec.Command(this.TarCommand, "xvif", tarfileName, fmt.Sprintf("--directory=%s", outputDir))
}

func (this TarClient) Tar(inputDir string) *exec.Cmd {
	// the -C flag ensures that the full filepath will not be included in the tar file
	return exec.Command(this.TarCommand, "cvf", "-", "-C", inputDir, ".")
}
