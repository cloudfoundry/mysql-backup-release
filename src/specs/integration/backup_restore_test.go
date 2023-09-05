package integration_test

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"code.cloudfoundry.org/tlsconfig/certtest"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/imdario/mergo"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry/specs/docker"
)

type TestCredentials struct {
	Username                 string
	Password                 string
	ClientTLS                CertificateInfo
	ServerTLS                CertificateInfo
	EnableMutualTLS          bool
	RequiredClientIdentities []string
}

type CertificateInfo struct {
	CA   string
	Cert string
	Key  string
}

var _ = Describe("BackupRestore", func() {
	var (
		backupServer           string
		backupServerPort       string
		backupServerDB         *sql.DB
		sessionID              string
		credentials            TestCredentials
		marshalledServerConfig string
		transport              *http.Transport
	)

	BeforeEach(func() {
		log.Println("Test starting")
		sessionID = uuid.New().String()

		credentials = TestCredentials{
			Username: uuid.New().String(),
			Password: uuid.New().String(),
		}

		log.Println("Generating test certificates")
		createBackupCertificates("backup-server."+sessionID, &credentials)
	})

	JustBeforeEach(func() {
		var err error

		marshalledServerConfig = backupServerConfig(credentials)

		log.Println("Starting backup server")
		backupServer, err = startStreamingMySQLBackupTool(
			"backup-server."+sessionID,
			marshalledServerConfig,
		)
		Expect(err).NotTo(HaveOccurred())

		backupServerPort, err = docker.ContainerPort(backupServer, "8081/tcp")
		Expect(err).NotTo(HaveOccurred())

		client := &http.Client{Transport: transport}

		Eventually(func() error {
			_, err := client.Get("https://127.0.0.1:" + backupServerPort)
			return err
		}, "1m", "1s").Should(Succeed())

		log.Println("streaming-mysql-backup-tool is now online")

		backupServerDB, err = docker.MySQLDB(backupServer, "3306/tcp")
		Expect(err).NotTo(HaveOccurred())

		Eventually(backupServerDB.Ping, "1m", "1s").Should(Succeed())

		log.Println("mysqld (backup-server) is now online")
	})

	AfterEach(func() {
		var errs error

		if err := docker.RemoveContainer("backup-server." + sessionID); err != nil {
			errs = multierror.Append(errs, err)
		}

		if err := docker.RemoveContainer("backup-client." + sessionID); err != nil {
			errs = multierror.Append(errs, err)
		}

		if err := docker.RemoveContainer("gpg-unpack." + sessionID); err != nil {
			errs = multierror.Append(errs, err)
		}

		if err := docker.RemoveContainer("mysql." + sessionID); err != nil {
			errs = multierror.Append(errs, err)
		}

		if err := docker.RemoveVolume("restore-data." + sessionID); err != nil {
			errs = multierror.Append(errs, err)
		}

		if err := docker.RemoveVolume("tmp-dir." + sessionID); err != nil {
			errs = multierror.Append(errs, err)
		}

		Expect(errs).ToNot(HaveOccurred())
	})

	setupBackupServer := func() {
		var err error

		By("Writing a value to backup-server mysql instance")
		log.Println("Generating some test data on backup-server")
		_, err = backupServerDB.Exec(`CREATE DATABASE foo`)
		Expect(err).NotTo(HaveOccurred())

		_, err = backupServerDB.Exec(`CREATE TABLE foo.t1 (id int primary key)`)
		Expect(err).NotTo(HaveOccurred())

		_, err = backupServerDB.Exec(`INSERT INTO foo.t1 VALUES (42)`)
		Expect(err).NotTo(HaveOccurred())

	}

	createBackup := func() {
		By("Generating a backup artifact using streaming-mysql-backup-client")
		log.Println("Creating backup artifact via streaming-mysql-backup-client")
		clientCfg := backupClientConfig(
			"backup-server."+sessionID,
			"secret",
			credentials,
		)

		Expect(runStreamingMySQLBackupClient(sessionID, clientCfg)).
			Error().NotTo(HaveOccurred(),
			"expected running streaming MySQL Backup Client to exit successfully")
		log.Println("Backup completed successfully")
	}

	restoreBackups := func() {
		By("Unpacking the backup artifact into a data directory")
		log.Println("Unpacking backup artifact")
		Expect(unpackBackupArtifact(sessionID)).To(Succeed())
		log.Println("Unpacking backup artifact completed successfully")
	}

	verifyData := func() {
		By("Starting a New MySQL using that restored data directory")
		mysqlContainer, err := startMySQLServer(sessionID)
		Expect(err).NotTo(HaveOccurred())

		mysqlDB, err := docker.MySQLDB(mysqlContainer, "3306/tcp")
		Expect(err).NotTo(HaveOccurred())

		Eventually(mysqlDB.Ping, "1m", "1s").Should(Succeed())
		log.Println("MySQL online with new data directory")

		By("Verifying the data was restored correctly")
		var value string

		Expect(
			mysqlDB.QueryRow(`SELECT id FROM foo.t1`).Scan(&value),
		).To(Succeed())

		Expect(value).To(Equal("42"))
		log.Println("Successfully validated data was restored.")
	}

	Context("when mutual TLS is not enabled", func() {
		BeforeEach(func() {
			credentials.EnableMutualTLS = false
			transport = &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			}

		})

		It("can successfully perform the backup / restore workflow", func() {
			setupBackupServer()
			createBackup()
			restoreBackups()
			verifyData()
		})

		It("can successfully perform the backup when broken artifacts fill up tmp directory", func() {
			setupBackupServer()
			clientCfg := backupClientConfig(
				"backup-server."+sessionID,
				"secret",
				credentials,
			)
			Expect(runStreamingMySQLBackupClientWithFullTmp(sessionID, clientCfg)).To(Succeed())
		})
	})

	Context("when mutual TLS is enabled", func() {
		BeforeEach(func() {
			credentials.EnableMutualTLS = true
			credentials.RequiredClientIdentities = []string{"client certificate"}

			cert, err := tls.X509KeyPair([]byte(credentials.ClientTLS.Cert), []byte(credentials.ClientTLS.Key))
			Expect(err).NotTo(HaveOccurred())

			transport = &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					Certificates:       []tls.Certificate{cert},
				},
			}
		})

		It("can successfully perform the backup / restore workflow", func() {
			setupBackupServer()
			createBackup()
			restoreBackups()
			verifyData()
		})

		Context("when untrusted client certificates are configured", func() {
			BeforeEach(func() {
				createUntrustedClientCertificates(&credentials)
			})

			It("backups will fail with a certificate error", func() {
				By("Generating a backup artifact using streaming-mysql-backup-client")
				log.Println("Creating backup artifact via streaming-mysql-backup-client")
				clientCfg := backupClientConfig(
					"backup-server."+sessionID,
					"secret",
					credentials,
				)

				err := runStreamingMySQLBackupClient(sessionID, clientCfg)
				Expect(err).To(HaveOccurred(),
					"expected running streaming MySQL Backup Client to fail, but it did not")

				output, err := docker.Command("logs", "backup-client."+sessionID)
				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(ContainSubstring(`Get \"https://backup-server.%s:8081/backup?format=xbstream\": remote error: tls: unknown certificate authority`, sessionID))
			})
		})
	})

})

func createBackupCertificates(serverName string, credentials *TestCredentials) {
	serverAuthority, err := certtest.BuildCA("server authority")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	serverCAPEM, err := serverAuthority.CertificatePEM()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	serverCertCtx, err := serverAuthority.BuildSignedCertificate(
		"server certificate",
		certtest.WithDomains(serverName),
	)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	serverCertPEM, serverKeyPEM, err := serverCertCtx.CertificatePEMAndPrivateKey()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	clientAuthority, err := certtest.BuildCA("client authority")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	clientCAPEM, err := clientAuthority.CertificatePEM()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	clientCertCtx, err := clientAuthority.BuildSignedCertificate("client certificate",
		certtest.WithDomains("client certificate"))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	clientCertPEM, clientKeyPEM, err := clientCertCtx.CertificatePEMAndPrivateKey()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	credentials.ClientTLS.CA = string(clientCAPEM)
	credentials.ClientTLS.Cert = string(clientCertPEM)
	credentials.ClientTLS.Key = string(clientKeyPEM)

	credentials.ServerTLS.CA = string(serverCAPEM)
	credentials.ServerTLS.Cert = string(serverCertPEM)
	credentials.ServerTLS.Key = string(serverKeyPEM)
}

func createUntrustedClientCertificates(credentials *TestCredentials) {
	untrustedAuthority, err := certtest.BuildCA("client authority")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	clientCertCtx, err := untrustedAuthority.BuildSignedCertificate("client certificate")
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	clientCertPEM, clientKeyPEM, err := clientCertCtx.CertificatePEMAndPrivateKey()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	credentials.ClientTLS.Cert = string(clientCertPEM)
	credentials.ClientTLS.Key = string(clientKeyPEM)
}

func backupServerConfig(credentials TestCredentials) string {
	cfg := map[string]interface{}{
		"PidFile": "/tmp/streaming-mysql-backup-tool.pid",
		"XtraBackup": map[string]string{
			"DefaultsFile": "/etc/my.cnf",
			"TmpDir":       "/tmp",
		},
		"BindAddress": ":8081",
		"TLS": map[string]interface{}{
			"ServerCert": credentials.ServerTLS.Cert,
			"ServerKey":  credentials.ServerTLS.Key,
		},
	}

	if credentials.EnableMutualTLS {
		mTlsConfig := map[string]interface{}{
			"TLS": map[string]interface{}{
				"ClientCA":                 credentials.ClientTLS.CA,
				"EnableMutualTLS":          true,
				"RequiredClientIdentities": credentials.RequiredClientIdentities,
			},
		}
		ExpectWithOffset(1, mergo.Merge(&cfg, mTlsConfig)).To(Succeed())
	} else {
		credentialConfig := map[string]interface{}{
			"Credentials": map[string]interface{}{
				"Username": credentials.Username,
				"Password": credentials.Password,
			},
		}
		ExpectWithOffset(1, mergo.Merge(&cfg, credentialConfig)).To(Succeed())
	}

	jsonBytes, err := json.Marshal(cfg)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	return string(jsonBytes)
}

func backupClientConfig(backupServerHost, encryptionPassword string, credentials TestCredentials) string {
	cfg := map[string]interface{}{
		"Instances": []map[string]string{
			{
				"Address": backupServerHost,
				"UUID":    "backup-server-uuid",
			},
		},
		"BackupServerPort":       8081,
		"BackupAllMasters":       false,
		"BackupFromInactiveNode": false,
		"TmpDir":                 "/tmp",
		"OutputDir":              "/backups",
		"SymmetricKey":           encryptionPassword,
		"TLS": map[string]interface{}{
			"EnableMutualTLS": false,
			"ServerCACert":    credentials.ServerTLS.CA,
		},
		"MetadataFields": map[string]interface{}{
			"compressed": "Y",
			"encrypted":  "Y",
		},
	}
	if credentials.EnableMutualTLS {
		mTlsConfig := map[string]interface{}{
			"TLS": map[string]interface{}{
				"ClientCert":      credentials.ClientTLS.Cert,
				"ClientKey":       credentials.ClientTLS.Key,
				"EnableMutualTLS": true,
			},
		}
		ExpectWithOffset(1, mergo.Merge(&cfg, mTlsConfig)).To(Succeed())
	} else {
		credentialConfig := map[string]interface{}{
			"Credentials": map[string]interface{}{
				"Username": credentials.Username,
				"Password": credentials.Password,
			},
		}
		ExpectWithOffset(1, mergo.Merge(&cfg, credentialConfig)).To(Succeed())
	}
	jsonBytes, err := json.Marshal(cfg)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	return string(jsonBytes)
}

func runStreamingMySQLBackupClientWithFullTmp(sessionID, config string) error {
	if _, err := docker.Command(
		"run",
		"--name=backup-client."+sessionID,
		"--network="+dockerNetwork,
		"--user=root",
		"--env=MYSQL_ALLOW_EMPTY_PASSWORD=1",
		"--env=CONFIG="+config,
		"--publish=3306",
		"--publish=8081",
		"--mount=type=tmpfs,destination=/tmp,tmpfs-mode=1777",
		"--volume="+streamingMySQLBackupClientBinPath+":/usr/local/bin/streaming-mysql-backup-client",
		"--volume=restore-data."+sessionID+":/backups",
		"--detach",
		DockerImage,
		"--user=root",
	); err != nil {
		return fmt.Errorf("failed to start streaming-mysql-backup-tool: %w", err)
	}

	// This fails, because /tmp will be full
	_, _ = fillUpTmp("backup-client." + sessionID)

	if _, err := docker.Command("exec", "--env=CONFIG="+config, "-i", "backup-client."+sessionID, "streaming-mysql-backup-client"); err != nil {
		return fmt.Errorf("failed to run streaming-mysql-backup-client: %w", err)
	}

	return nil
}

func runStreamingMySQLBackupClient(sessionID, config string) error {
	_, err := docker.Command(
		"run",
		"--name=backup-client."+sessionID,
		"--network="+dockerNetwork,
		"--user=root",
		"--env=MYSQL_ALLOW_EMPTY_PASSWORD=1",
		"--env=CONFIG="+config,
		"--publish=3306",
		"--publish=8081",
		"--volume="+streamingMySQLBackupClientBinPath+":/usr/local/bin/streaming-mysql-backup-client",
		"--volume=restore-data."+sessionID+":/backups",
		"--entrypoint=streaming-mysql-backup-client",
		DockerImage,
	)

	return err
}

func unpackBackupArtifact(sessionID string) error {
	_, err := docker.Command(
		"run",
		"--name=gpg-unpack."+sessionID,
		"--network="+dockerNetwork,
		"--user=root",
		"--env=GPG_PASSPHRASE=secret",
		"--env=ARTIFACTS_PATH=/restore/",
		"--env=DATA_DIRECTORY=/restore/",
		"--publish=3306",
		"--publish=8081",
		"--volume=restore-data."+sessionID+":/restore",
		"--volume="+filepath.Join(fixturesPath, "restore-artifact.sh")+":/usr/local/bin/restore-artifact",
		"--entrypoint=restore-artifact",
		DockerImage,
		"--user=root",
	)

	return err
}

func startStreamingMySQLBackupTool(containerName, config string) (string, error) {
	return docker.Command(
		"run",
		"--name="+containerName,
		"--network="+dockerNetwork,
		"--user=root",
		"--env=MYSQL_ALLOW_EMPTY_PASSWORD=1",
		"--env=CONFIG="+config,
		"--publish=3306",
		"--publish=8081",
		"--volume="+streamingMySQLBackupToolBinPath+":/usr/local/bin/streaming-mysql-backup-tool",
		"--volume="+filepath.Join(fixturesPath, "start-backup-server.sh")+":/docker-entrypoint-initdb.d/start-backup-server.sh",
		"--detach",
		DockerImage,
		"--user=root",
	)
}

func startMySQLServer(sessionID string) (string, error) {
	return docker.Command(
		"run",
		"--name=mysql."+sessionID,
		"--network="+dockerNetwork,
		"--user=root",
		"--env=MYSQL_ALLOW_EMPTY_PASSWORD=1",
		"--publish=3306",
		"--volume=restore-data."+sessionID+":/var/lib/mysql",
		"--detach",
		DockerImage,
		"--user=root",
	)
}

func runCmdInDocker(container string, cmd []string) (string, error) {
	args := append([]string{
		"exec",
		"-it",
		container,
	}, cmd...)

	return docker.Command(args...)
}

// In order to simulate a full tmp directory which result from failed partial backups,
// we fill up the tmp directory.

func fillUpTmp(container string) (string, error) {
	mkdirCmd := []string{
		"mkdir",
		"-p",
		"/tmp/mysql-backup-test/",
	}
	_, err := runCmdInDocker(container, mkdirCmd)
	if err != nil {
		return "", err
	}

	ddCmd := []string{
		"dd",
		"if=/dev/zero",
		"of=/tmp/mysql-backup-test/placeholder-2 bs=128M",
	}

	return runCmdInDocker(container, ddCmd)
}
