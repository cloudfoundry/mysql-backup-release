package lifecycle_test

import (
	"log"
	"os"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func boshStartAll() bool {
	log.Println()
	session, err := runBoshCommand("start")
	Expect(err).NotTo(HaveOccurred())

	session.Wait("5m")

	log.Println()

	return session.ExitCode() == 0
}

func runBoshCommand(args ...string) (*gexec.Session, error) {
	defaultArgs := []string{
		"--non-interactive",
		"--deployment=" + os.Getenv("BOSH_DEPLOYMENT"),
	}

	cmd := exec.Command("bosh",
		append(
			defaultArgs,
			args...,
		)...,
	)

	log.Printf("$ %s", strings.Join(cmd.Args, " "))

	return gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
}

var _ = Describe("Streaming MySQL Backup Tool Lifecycle", func() {
	When("streaming-mysql-backup-tool is shutdown", func() {
		BeforeEach(func() {
			shutdownBackupTool, err := runBoshCommand(
				"ssh",
				"mysql/0",
				"-c",
				"sudo /var/vcap/bosh/bin/monit stop streaming-mysql-backup-tool",
			)
			Expect(err).NotTo(HaveOccurred())

			Eventually(shutdownBackupTool, "5m").
				Should(gexec.Exit(0),
					"Expected monit to stop streaming-mysql-backup-tool")
		})

		AfterEach(func() {
			Expect(boshStartAll()).To(BeTrue())
		})

		// We can remove this test/test suite when mysql-backup-release has moved to BPM
		It("removes its PID file", func() {
			Eventually(checkPidFileIsGone, "30s", "2s").
				Should(BeTrue(),
					"Expected streaming-mysql-backup-tool pid file to be removed but it was not")
		})
	})
})

func checkPidFileIsGone() bool {
	checkPidFile, err := runBoshCommand(
		"ssh",
		"mysql/0",
		"-c",
		"! [[ -e /var/vcap/sys/run/streaming-mysql-backup-tool/streaming-mysql-backup-tool.pid ]]",
	)
	Expect(err).NotTo(HaveOccurred())

	checkPidFile.Wait("5m")

	return checkPidFile.ExitCode() == 0
}
