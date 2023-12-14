package e2e_tests

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/bosh"
	"e2e-tests/utilities/cmd"
	"e2e-tests/utilities/credhub"
)

var _ = Describe("Streaming MySQL Backup Tool", Ordered, Label("backup-restore"), func() {

	var (
		deploymentName string
		backupTmpDir   string
		firstInstance  string
	)

	BeforeAll(func() {
		deploymentName = "mysql-backup-release-backup-restore-test" + uuid.New().String()
		var opsFileArgument []string
		if opsFile := os.Getenv("PXC_OPS_FILE_TO_APPLY"); opsFile != "" {
			opsFileArgument = append(opsFileArgument, "--ops-file="+filepath.Join("operations", opsFile))
		}

		Expect(cmd.RunCustom(
			cmd.Setup(
				cmd.WithCwd("../.."),
				cmd.WithEnv("DEPLOYMENT_NAME="+deploymentName),
			),
			"./scripts/deploy-engine", opsFileArgument...
		)).To(Succeed())

		instances, err := bosh.Instances(deploymentName, bosh.MatchByInstanceGroup("mysql"))
		Expect(err).NotTo(HaveOccurred())
		Expect(instances).NotTo(BeEmpty())

		firstInstance = instances[len(instances)-1].Instance
	})

	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			return
		}

		Expect(bosh.DeleteDeployment(deploymentName)).To(Succeed())
	})

	When("mutual TLS is enabled", func() {

		BeforeEach(func() {
			var err error
			By("Creating a /tmp/mysql-backups directory on the local machine")
			backupTmpDir, err = os.MkdirTemp("", "mysql-backups")
			Expect(err).NotTo(HaveOccurred())
		})

		It("can successfully perform the backup / restore workflow", func() {

			By("Deleting any previous backups")
			_, err := bosh.RemoteCommand(deploymentName,
				"backup-prepare",
				fmt.Sprintf("sudo rm -f %s*", backupDir))
			Expect(err).NotTo(HaveOccurred())

			By("Writing some test data")
			_, err = bosh.RemoteCommand(deploymentName,
				firstInstance,
				fmt.Sprintf("sudo mysql --defaults-file=%s -e 'create database if not exists test;'", myLoginCnfFilePath))
			Expect(err).NotTo(HaveOccurred())

			_, err = bosh.RemoteCommand(deploymentName,
				firstInstance,
				fmt.Sprintf("sudo mysql --defaults-file=%s -e 'create table if not exists test.foo (id int primary key);'", myLoginCnfFilePath))
			Expect(err).NotTo(HaveOccurred())

			_, err = bosh.RemoteCommand(deploymentName,
				firstInstance,
				fmt.Sprintf("sudo mysql --defaults-file=%s -e 'insert into test.foo values (42);'", myLoginCnfFilePath))
			Expect(err).NotTo(HaveOccurred())

			By("Generating a backup artifact using streaming-mysql-backup-client")
			_, err = bosh.RemoteCommand(deploymentName, "backup-prepare",
				"sudo /var/vcap/jobs/streaming-mysql-backup-client/bin/client")
			Expect(err).NotTo(HaveOccurred())

			By("Fetching the backup artifact name")
			backupArtifactName, err := bosh.RemoteCommand(deploymentName, "backup-prepare",
				fmt.Sprintf("sudo ls %s*.gpg | head -1 | awk '{print $1}' | xargs -n 1 basename", backupDir))
			Expect(err).NotTo(HaveOccurred())

			By("Fetching the symmetric key")
			symmetricKey, err := credhub.GetCredhubPassword("/" + deploymentName + "/cf_mysql_backup_symmetric_key")
			Expect(err).NotTo(HaveOccurred())

			By("Fetching the backup artifact")
			Expect(bosh.Scp(deploymentName,
				fmt.Sprintf("backup-prepare:%s", backupDir+backupArtifactName),
				filepath.Join(backupTmpDir, backupArtifactName), "-r", "-l", "root")).To(Succeed())

			By("Copying the backup artifact")
			Expect(
				bosh.Scp(deploymentName,
					filepath.Join(backupTmpDir, backupArtifactName),
					firstInstance+":/tmp/", "-l", "root "),
			).Should(Succeed())

			By("Stopping MySQL")
			Expect(bosh.Stop(deploymentName, firstInstance, "-n")).To(Succeed())

			By("Decrypting the backup artifact")
			_, err = bosh.RemoteCommand(deploymentName, firstInstance,
				"gpg --batch --yes --no-tty --compress-algo zip --cipher-algo AES256 --output "+
					"/tmp/mysql-backup.tar --passphrase "+symmetricKey+
					" --decrypt /tmp/"+backupArtifactName,
			)
			Expect(err).NotTo(HaveOccurred())

			By("Deleting the MySQL datadir")
			_, err = bosh.RemoteCommand(deploymentName, firstInstance, fmt.Sprintf("sudo rm -rf %s/*", mysqlDatdir))
			Expect(err).NotTo(HaveOccurred())

			By("Restoring MySQL from the backup")
			_, err = bosh.RemoteCommand(deploymentName, firstInstance, fmt.Sprintf("sudo tar -xvf /tmp/mysql-backup.tar -C %s/", mysqlDatdir))
			Expect(err).NotTo(HaveOccurred())

			_, err = bosh.RemoteCommand(deploymentName, firstInstance, "sudo rm /tmp/mysql-backup.tar")
			Expect(err).NotTo(HaveOccurred())

			_, err = bosh.RemoteCommand(deploymentName, firstInstance, fmt.Sprintf("sudo chown -R vcap:vcap %s", mysqlDatdir))
			Expect(err).NotTo(HaveOccurred())

			By("Starting MySQL")
			Expect(bosh.Start(deploymentName, firstInstance, "-n")).To(Succeed())

			By("Verifying the restored data")
			out, err := bosh.RemoteCommand(deploymentName,
				firstInstance,
				fmt.Sprintf("sudo mysql --defaults-file=%s -sse 'SELECT * from test.foo;'", myLoginCnfFilePath))
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(Equal("42"))
		})

		AfterEach(func() {
			By("Deleting the /tmp/mysql-backups directory on the local machine")
			Expect(os.RemoveAll(backupTmpDir)).To(Succeed())
		})
	})
})
