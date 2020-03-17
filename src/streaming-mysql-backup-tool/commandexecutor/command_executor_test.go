package commandexecutor_test

import (
	"bytes"
	"errors"
	"os/exec"
	"time"

	"code.cloudfoundry.org/lager"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	. "streaming-mysql-backup-tool/commandexecutor"
	"streaming-mysql-backup-tool/commandexecutor/commandexecutorfakes"
)

var _ = Describe("CommandExecutor", func() {
	var (
		logger     lager.Logger
		fakeWriter *commandexecutorfakes.FakeWriter

		stdoutWriter bytes.Buffer
		stderrWriter bytes.Buffer
		cmd          *exec.Cmd

		expectedOutputStr string
		expectedOutput    []byte
	)

	BeforeEach(func() {
		expectedOutputStr = "my_output"
		logger = lager.NewLogger("test")
		fakeWriter = new(commandexecutorfakes.FakeWriter)

		cmd = exec.Command("echo", "-n", expectedOutputStr)
	})

	It("Return known output", func() {
		executor := NewCommandExecutor(cmd, &stdoutWriter, &stderrWriter, logger)

		err := executor.Run()
		Expect(err).ShouldNot(HaveOccurred())

		expectedOutput := []byte(expectedOutputStr)
		actualOutput := stdoutWriter.Bytes()
		Expect(actualOutput).To(Equal(expectedOutput))
	})

	Context("when io.Copy finishes after command has exited", func() {
		var (
			finishedWriting bool
		)

		BeforeEach(func() {
			expectedOutput = []byte(expectedOutputStr)
			bytesSoFar := 0
			expectedByteCount := len(expectedOutput)
			finishedWriting = false

			fakeWriter.WriteStub = func(b []byte) (n int, err error) {
				bytesSoFar += len(b)
				// sleep to force io.Copy to finish after command has already exited
				if bytesSoFar == expectedByteCount {
					time.Sleep(2 * time.Second)
					finishedWriting = true
				}

				return len(b), nil
			}
		})

		It("waits for io.Copy to finish", func() {
			executor := NewCommandExecutor(cmd, fakeWriter, fakeWriter, logger)

			err := executor.Run()
			Expect(err).ShouldNot(HaveOccurred())

			Expect(finishedWriting).To(BeTrue(), "Expected Run() to wait for io.Copy to finish")
		})
	})

	Context("when the HTTP response writer returns an error", func() {
		BeforeEach(func() {
			fakeWriter.WriteReturns(0, errors.New("fake-error"))
		})

		It("Returns the error from the writer", func() {
			executor := NewCommandExecutor(cmd, fakeWriter, fakeWriter, logger)
			err := executor.Run()

			Expect(err).To(MatchError("fake-error"))
		})

		It("kills the command", func(done Done) {
			cmd = exec.Command("yes") //yes prints 'y' forever
			executor := NewCommandExecutor(cmd, fakeWriter, fakeWriter, logger)

			Expect(executor.Run()).To(MatchError("fake-error"))
			close(done)
		}, 5)
	})

	Context("when the command has nonzero exit code", func() {
		BeforeEach(func() {
			expectedOutput = []byte("cat: nonexistentfile: No such file or directory\n")

			cmd = exec.Command("cat", "nonexistentfile")
		})

		It("Copies any error output to the stderr writer", func() {
			executor := NewCommandExecutor(cmd, &stdoutWriter, &stderrWriter, logger)
			err := executor.Run()
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Command did not complete"))
			actualOutput := stderrWriter.Bytes()
			Expect(actualOutput).To(Equal(expectedOutput))
		})
	})

	Context("when the command fails on I/O but ignores sigint", func() {
		var (
			badCmdPath string
			badCmd     *exec.Cmd
		)

		BeforeEach(func() {
			var err error
			badCmdPath, err = gexec.Build("fixtures/block_sigint.go")
			Expect(err).ToNot(HaveOccurred())
			badCmd = exec.Command(badCmdPath)
		})

		AfterEach(func() {
			gexec.CleanupBuildArtifacts()
		})

		It("still terminates the command eventually", func(done Done) {
			fakeWriter.WriteReturns(-1, errors.New("fake-error"))
			executor := NewCommandExecutor(badCmd, fakeWriter, &stderrWriter, logger)
			defer func() {
				if badCmd.Process != nil {
					badCmd.Process.Kill()
				}
			}()

			Expect(executor.Run()).ToNot(Succeed())
			close(done)
		}, 10.0)
	})
})
