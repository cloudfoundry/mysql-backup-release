package e2e_tests

import (
	"database/sql"

	"e2e-tests/utilities/bosh"
	"e2e-tests/utilities/cmd"
	"e2e-tests/utilities/credhub"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Streaming MySQL Backup Tool", Ordered, Label("backup-restore"), func() {

	var (
		db             *sql.DB
		deploymentName string
	)

	BeforeAll(func() {
		deploymentName = "mysql-backup-release-backup-restore-test" + uuid.New().String()

		Expect(cmd.RunCustom(
			cmd.Setup(
				cmd.WithCwd("../.."),
				cmd.WithEnv("DEPLOYMENT_NAME="+deploymentName),
			),
			"./scripts/deploy-engine",
		)).To(Succeed())

		proxyIPs, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("proxy"))
		Expect(err).NotTo(HaveOccurred())
		Expect(proxyIPs).To(HaveLen(2))

		db, err = sql.Open("mysql", "test-admin:integration-tests@tcp("+proxyIPs[0]+")/?tls=skip-verify")
		Expect(err).NotTo(HaveOccurred())
		db.SetMaxIdleConns(0)
		db.SetMaxOpenConns(1)
	})

	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			return
		}

		//Expect(bosh.DeleteDeployment(deploymentName)).To(Succeed())
	})

	When("mutual TLS is not enabled", func() {

		It("can successfully perform the backup / restore workflow", func() {
			By("Deleting any previous backups")
			_, err := bosh.RemoteCommand(deploymentName,
				"backup-prepare",
				"sudo rm -f /var/vcap/store/mysql-backups/*")
			Expect(err).NotTo(HaveOccurred())

			By("Writing some test data")
			_, err = bosh.RemoteCommand(deploymentName,
				"mysql/0",
				"sudo mysql --defaults-file=/var/vcap/jobs/pxc-mysql/config/mylogin.cnf -e 'create database if not exists test;'")
			Expect(err).NotTo(HaveOccurred())

			_, err = bosh.RemoteCommand(deploymentName,
				"mysql/0",
				"sudo mysql --defaults-file=/var/vcap/jobs/pxc-mysql/config/mylogin.cnf -e 'create table if not exists test.foo (id int primary key);'")
			Expect(err).NotTo(HaveOccurred())

			_, err = bosh.RemoteCommand(deploymentName,
				"mysql/0",
				"sudo mysql --defaults-file=/var/vcap/jobs/pxc-mysql/config/mylogin.cnf -e 'insert into test.foo values (42);'")
			Expect(err).NotTo(HaveOccurred())

			By("Generating a backup artifact using streaming-mysql-backup-client")
			_, err = bosh.RemoteCommand(deploymentName, "backup-prepare",
				"sudo /var/vcap/jobs/streaming-mysql-backup-client/bin/client")
			Expect(err).NotTo(HaveOccurred())

			By("Fetching the backup artifact name")
			backupArtifactName, err := bosh.RemoteCommand(deploymentName, "backup-prepare",
				"sudo ls /var/vcap/store/mysql-backups/*.gpg | head -1 | awk '{print $1}' | xargs -n 1 basename")
			Expect(err).NotTo(HaveOccurred())


			By("Fetching the symmetric key")
			symmetricKey, err := credhub.GetCredhubPassword("/" + deploymentName + "/cf_mysql_backup_symmetric_key")
			Expect(err).NotTo(HaveOccurred())

			By("Fetching the backup artifact")
			Expect(bosh.Scp(deploymentName,
				"backup-prepare:/var/vcap/store/mysql-backups/"+backupArtifactName,
				"/tmp/mysql-backups/"+backupArtifactName, "-r", "-l", "root")).To(Succeed())


			By("Copying the backup artifact back to mysql/0")
			Eventually(
				bosh.Scp(deploymentName, "/tmp/mysql-backups/"+backupArtifactName, "mysql/0:/tmp/", "-l", "root "),
				"10m",
			).Should(Succeed())

			By("Stopping MySQL")
			Expect(bosh.Stop(deploymentName, "mysql/0", "-n")).To(Succeed())

			By("Decrypting the backup artifact")
			//gpgCmd := []string{"gpg", "--batch", "--yes", "--no-tty",
			//	"--compress-algo", "zip", "--cipher-algo", "AES256",
			//	"--output", "/tmp/mysql-backup.tar", "--passphrase",
			//	symmetricKey, "--decrypt", "/tmp/"+backupArtifactName}
			bosh.RemoteCommand(deploymentName, "mysql/0",
				"gpg --batch --yes --no-tty --compress-algo zip --cipher-algo AES256 --output " +
				"/tmp/mysql-backup.tar --passphrase " + symmetricKey +
				" --decrypt /tmp/"+backupArtifactName,
			)

			By("Deleting the MySQL datadir")
			bosh.RemoteCommand(deploymentName, "mysql/0", "sudo rm -rf /var/vcap/store/pxc-mysql/*")

			By("Restoring MySQL from the backup")
			_, err = bosh.RemoteCommand(deploymentName, "mysql/0", "sudo tar -xvf /tmp/mysql-backup.tar -C /var/vcap/store/pxc-mysql/")
			Expect(err).NotTo(HaveOccurred())

			_, err = bosh.RemoteCommand(deploymentName, "mysql/0", "sudo rm /tmp/mysql-backup.tar")
			Expect(err).NotTo(HaveOccurred())

			_, err = bosh.RemoteCommand(deploymentName, "mysql/0", "sudo chown -R vcap:vcap /var/vcap/store/pxc-mysql")
			Expect(err).NotTo(HaveOccurred())

			By("Starting MySQL")
			Expect(bosh.Start(deploymentName, "mysql/0", "-n")).To(Succeed())

			By("Verifying the restored data")
			out, err := bosh.RemoteCommand(deploymentName,
				"mysql/0",
				"sudo mysql --defaults-file=/var/vcap/jobs/pxc-mysql/config/mylogin.cnf -sse 'SELECT * from test.foo;'")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(Equal("42"))
		})
	})
})
