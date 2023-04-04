package commandexecutor

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"code.cloudfoundry.org/lager/v3"
)

type CommandExecutor struct {
	command      *exec.Cmd
	stdoutWriter io.Writer
	stderrWriter io.Writer
	logger       lager.Logger
}

func NewCommandExecutor(command *exec.Cmd, stdoutWriter io.Writer, stderrWriter io.Writer, logger lager.Logger) CommandExecutor {
	return CommandExecutor{
		command:      command,
		stdoutWriter: stdoutWriter,
		stderrWriter: stderrWriter,
		logger:       logger,
	}
}

func (c CommandExecutor) Run() error {
	c.command.Stderr = c.stderrWriter
	stdout, err := c.command.StdoutPipe()

	if err != nil {
		c.logger.Error("Cannot get a Stdout Pipe", err)
		return err
	}

	startErr := c.command.Start()
	if startErr != nil {
		c.logger.Error("Cannot start command", startErr)
		return startErr
	}

	_, stdoutIoErr := io.Copy(c.stdoutWriter, stdout)
	if stdoutIoErr != nil {
		// we don't care about errs from canceled commands
		_ = c.command.Process.Signal(os.Interrupt)
		_ = stdout.Close()
		_ = c.command.Wait()
		return stdoutIoErr
	}

	err = c.command.Wait()
	if err != nil {
		return errors.New(fmt.Sprintf("Command did not complete successfully: %s", err.Error()))
	}

	return nil
}
