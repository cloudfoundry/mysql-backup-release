package e2e_tests

import (
	"log"
	"os/exec"
	"strings"

	"e2e-tests/utilities/bosh"
	"e2e-tests/utilities/cmd"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func boshStartAll(deployment string ) bool {
	log.Println()
	session, err := runBoshCommand(deployment,"start")
	Expect(err).NotTo(HaveOccurred())

	session.Wait("5m")

	log.Println()

	return session.ExitCode() == 0
}

func runBoshCommand(deployment string, args ...string) (*gexec.Session, error) {
	defaultArgs := []string{
		"--non-interactive",
		"--deployment=" + deployment,
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

var _ = Describe("Streaming MySQL Backup Tool Lifecycle", Ordered, Label("life-cycle"), func() {

	var (
		deploymentName string
	)

	BeforeAll(func() {
		deploymentName = "mysql-backup-release-lifecycle-test-" + uuid.New().String()

		Expect(cmd.RunCustom(
			cmd.Setup(
				cmd.WithCwd("../.."),
				cmd.WithEnv("DEPLOYMENT_NAME="+deploymentName),
			),
			"./scripts/deploy-engine",
		)).To(Succeed())

		instanceAddresses, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("mysql"))
		Expect(err).NotTo(HaveOccurred())
		Expect(instanceAddresses).NotTo(BeEmpty(), `Expected a set of IP addresses to be computed for the deployment, but it was missing`)
	})


	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			return
		}

		Expect(bosh.DeleteDeployment(deploymentName)).To(Succeed())
	})

	When("streaming-mysql-backup-tool is shutdown", func() {
		BeforeEach(func() {
			shutdownBackupTool, err := runBoshCommand(
				deploymentName,
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
			Expect(boshStartAll(deploymentName)).To(BeTrue())
		})

		// We can remove this test/test suite when mysql-backup-release has moved to BPM
		It("removes its PID file", func() {
			Eventually(checkPidFileIsGone(deploymentName), "30s", "2s").
				Should(BeTrue(),
					"Expected streaming-mysql-backup-tool pid file to be removed but it was not")
		})
	})
})

func checkPidFileIsGone(deployment string) bool {
	checkPidFile, err := runBoshCommand(
		deployment,
		"ssh",
		"mysql/0",
		"-c",
		"! [[ -e /var/vcap/sys/run/streaming-mysql-backup-tool/streaming-mysql-backup-tool.pid ]]",
	)
	Expect(err).NotTo(HaveOccurred())

	checkPidFile.Wait("5m")

	return checkPidFile.ExitCode() == 0
}
