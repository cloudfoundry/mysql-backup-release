package integration_test

import (
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"

	"code.cloudfoundry.org/tlsconfig/certtest"
	docker "github.com/fsouza/go-dockerclient"
	"github.com/google/uuid"
	"github.com/hashicorp/go-multierror"
	"github.com/imdario/mergo"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/pivotal/mysql-test-utils/dockertest"
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
		backupServer           *docker.Container
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

		_, err := dockerClient.CreateVolume(docker.CreateVolumeOptions{
			Name: "restore-data." + sessionID,
		})

		_, err = dockerClient.CreateVolume(docker.CreateVolumeOptions{
			Name:   "tmp-dir." + sessionID,
			Driver: "local",
			DriverOpts: map[string]string{
				"type":   "tmpfs",
				"device": "tmpfs",
				"o":      "size=500m",
			},
		})

		Expect(err).NotTo(HaveOccurred())
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

		backupServerPort = dockertest.HostPort("8081/tcp", backupServer)

		client := &http.Client{
			Transport: transport,
		}

		Eventually(func() error {
			_, err := client.Get("https://127.0.0.1:" + backupServerPort)
			return err
		}, "1m", "1s").Should(Succeed())

		log.Println("streaming-mysql-backup-tool is now online")

		backupServerDB, err = dockertest.ContainerDBConnection(backupServer, "3306/tcp")
		Expect(err).NotTo(HaveOccurred())

		Eventually(backupServerDB.Ping, "1m", "1s").Should(Succeed())

		log.Println("mysqld (backup-server) is now online")
	})

	AfterEach(func() {
		var errs error

		if err := dockerClient.RemoveContainer(docker.RemoveContainerOptions{ID: "backup-server." + sessionID, RemoveVolumes: true, Force: true}); err != nil {
			if _, ok := err.(*docker.NoSuchContainer); !ok {
				errs = multierror.Append(errs, err)
			}
		}

		if err := dockerClient.RemoveContainer(docker.RemoveContainerOptions{ID: "backup-client." + sessionID, RemoveVolumes: true, Force: true}); err != nil {
			if _, ok := err.(*docker.NoSuchContainer); !ok {
				errs = multierror.Append(errs, err)
			}
		}

		if err := dockerClient.RemoveContainer(docker.RemoveContainerOptions{ID: "gpg-unpack." + sessionID, RemoveVolumes: true, Force: true}); err != nil {
			if _, ok := err.(*docker.NoSuchContainer); !ok {
				errs = multierror.Append(errs, err)
			}
		}

		if err := dockerClient.RemoveContainer(docker.RemoveContainerOptions{ID: "mysql." + sessionID, RemoveVolumes: true, Force: true}); err != nil {
			if _, ok := err.(*docker.NoSuchContainer); !ok {
				errs = multierror.Append(errs, err)
			}
		}

		if err := dockerClient.RemoveVolumeWithOptions(docker.RemoveVolumeOptions{Name: "restore-data." + sessionID}); err != nil {
			if err == docker.ErrNoSuchVolume {
				errs = multierror.Append(errs, err)
			}
		}

		if err := dockerClient.RemoveVolumeWithOptions(docker.RemoveVolumeOptions{Name: "tmp-dir." + sessionID}); err != nil {
			if err == docker.ErrNoSuchVolume {
				errs = multierror.Append(errs, err)
			}
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
		var err error

		By("Generating a backup artifact using streaming-mysql-backup-client")
		log.Println("Creating backup artifact via streaming-mysql-backup-client")
		clientCfg := backupClientConfig(
			"backup-server."+sessionID,
			"secret",
			credentials,
		)

		exitStatus, err := runStreamingMySQLBackupClient(sessionID, clientCfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(exitStatus).To(BeZero(), "expected running streaming MySQL Backup Client to exit successfully")
		log.Println("Backup completed successfully")
	}

	restoreBackups := func() {
		By("Unpacking the backup artifact into a data directory")
		log.Println("Unpacking backup artifact")
		unpackExitStatus, err := unpackBackupArtifact(sessionID)
		Expect(err).NotTo(HaveOccurred())
		Expect(unpackExitStatus).To(BeZero())
		log.Println("Unpacking backup artifact completed successfully")

	}

	verifyData := func() {
		By("Starting a New MySQL using that restored data directory")
		mysqlContainer, err := startMySQLServer(sessionID)
		Expect(err).NotTo(HaveOccurred())

		mysqlDB, err := dockertest.ContainerDBConnection(mysqlContainer, "3306/tcp")
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
			exitStatus, err := runStreamingMySQLBackupClientWithFullTmp(sessionID, clientCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(exitStatus).To(BeZero(), "expected running streaming MySQL Backup Client to exit successfully")
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

				exitStatus, err := runStreamingMySQLBackupClient(sessionID, clientCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(exitStatus).ToNot(BeZero(),
					"expected running streaming MySQL Backup Client to fail, but it did not")

				buf := gbytes.NewBuffer()
				err = dockerClient.Logs(docker.LogsOptions{
					Container:    "backup-client." + sessionID,
					OutputStream: buf,
					ErrorStream:  buf,
					Stdout:       true,
					Stderr:       true,
				})
				Expect(err).NotTo(HaveOccurred())

				Expect(buf).To(gbytes.Say(`Get "?https://backup-server.%s:8081/backup"?: remote error: tls: bad certificate`, sessionID))
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
		"Command": "xtrabackup --user=root --backup --stream=tar --target-dir=/tmp",
		"Port":    8081,
		"TLS": map[string]interface{}{
			"ServerCert": string(credentials.ServerTLS.Cert),
			"ServerKey":  string(credentials.ServerTLS.Key),
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

func runStreamingMySQLBackupClientWithFullTmp(sessionID, config string) (exitStatus int, err error) {
	container, err := dockertest.RunContainer(
		dockerClient,
		"backup-client."+sessionID,
		dockertest.WithImage(DockerImage),
		dockertest.WithNetwork(dockerNetwork),
		dockertest.WithUser("root"),
		dockertest.AddEnvVars(
			"MYSQL_ALLOW_EMPTY_PASSWORD=1",
			"CONFIG="+config,
		),
		dockertest.AddBinds(
			streamingMySQLBackupClientBinPath+":/usr/local/bin/streaming-mysql-backup-client:delegated",
			"restore-data."+sessionID+":/backups",
			"tmp-dir."+sessionID+":/tmp",
		),
		dockertest.WithCmd("--user=root"),
	)

	if err != nil {
		return -1, err
	}

	fillUpTmp(container)

	exec, err := dockerClient.CreateExec(docker.CreateExecOptions{
		Container: container.ID,
		Cmd: []string{
			"streaming-mysql-backup-client",
			"--config=" + config,
		},
		AttachStdout: true,
		AttachStderr: true,
	})

	if err != nil {
		return -1, err
	}

	result, err := dockertest.RunExec(dockerClient, exec)
	if err != nil {
		return -1, err
	}
	return result.ExitCode, nil
}

func runStreamingMySQLBackupClient(sessionID, config string) (exitStatus int, err error) {
	container, err := dockertest.RunContainer(
		dockerClient,
		"backup-client."+sessionID,
		dockertest.WithImage(DockerImage),
		dockertest.WithNetwork(dockerNetwork),
		dockertest.WithUser("root"),
		dockertest.AddEnvVars(
			"CONFIG="+config,
		),
		dockertest.AddBinds(
			streamingMySQLBackupClientBinPath+":/usr/local/bin/streaming-mysql-backup-client:delegated",
			"restore-data."+sessionID+":/backups",
		),
		dockertest.WithEntrypoint("streaming-mysql-backup-client"),
		dockertest.WithCmd("--config="+config),
	)

	if err != nil {
		return -1, err
	}

	return dockerClient.WaitContainer(container.ID)
}

func unpackBackupArtifact(sessionID string) (exitStatus int, err error) {
	container, err := dockertest.RunContainer(
		dockerClient,
		"gpg-unpack."+sessionID,
		dockertest.WithImage(DockerImage),
		dockertest.WithNetwork(dockerNetwork),
		dockertest.WithUser("root"),
		dockertest.AddEnvVars(
			"GPG_PASSPHRASE=secret",
			"ARTIFACTS_PATH=/restore/",
			"DATA_DIRECTORY=/restore/",
		),
		dockertest.AddBinds(
			"restore-data."+sessionID+":/restore",
			filepath.Join(fixturesPath, "restore-artifact.sh")+":/usr/local/bin/restore-artifact",
		),
		dockertest.WithEntrypoint("restore-artifact"),
	)

	if err != nil {
		return -1, err
	}

	return dockerClient.WaitContainer(container.ID)
}

func startStreamingMySQLBackupTool(containerName, config string) (*docker.Container, error) {
	return dockertest.RunContainer(
		dockerClient,
		containerName,
		dockertest.WithImage(DockerImage),
		dockertest.WithNetwork(dockerNetwork),
		dockertest.WithUser("root"),
		dockertest.AddEnvVars(
			"MYSQL_ALLOW_EMPTY_PASSWORD=1",
			"CONFIG="+config,
		),
		dockertest.AddExposedPorts("3306/tcp", "8081/tcp"),
		dockertest.AddBinds(
			streamingMySQLBackupToolBinPath+":/usr/local/bin/streaming-mysql-backup-tool",
			filepath.Join(fixturesPath, "start-backup-server.sh")+":/docker-entrypoint-initdb.d/start-backup-server.sh",
		),
		dockertest.WithCmd("--user=root"),
	)
}

func startMySQLServer(sessionID string) (*docker.Container, error) {
	return dockertest.RunContainer(
		dockerClient,
		"mysql."+sessionID,
		dockertest.WithImage(DockerImage),
		dockertest.WithNetwork(dockerNetwork),
		dockertest.WithUser("root"),
		dockertest.AddEnvVars(
			"MYSQL_ALLOW_EMPTY_PASSWORD=1",
		),
		dockertest.AddExposedPorts("3306/tcp"),
		dockertest.AddBinds(
			"restore-data."+sessionID+":/var/lib/mysql",
		),
		dockertest.WithCmd("--user=root"),
	)
}

func runCmdInDocker(container *docker.Container, cmd []string) (*dockertest.ExecResult, error) {
	exec, err := dockerClient.CreateExec(docker.CreateExecOptions{
		Container:    container.ID,
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return nil, err
	}

	return dockertest.RunExec(dockerClient, exec)
}

// In order to simulate a full tmp directory which result from failed partial backups,
// we fill up the tmp directory.

func fillUpTmp(container *docker.Container) (*dockertest.ExecResult, error) {
	mkdirCmd := []string{
		"mkdir",
		"-p",
		"/tmp/mysql-backup-test/",
	}
	_, err := runCmdInDocker(container, mkdirCmd)
	if err != nil {
		return nil, err
	}

	ddCmd := []string{
		"dd",
		"if=/dev/zero",
		"of=/tmp/mysql-backup-test/placeholder-2 bs=128M",
	}

	return runCmdInDocker(container, ddCmd)
}
