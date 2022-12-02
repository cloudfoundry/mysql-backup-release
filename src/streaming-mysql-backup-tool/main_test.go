package main_test

import (
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"code.cloudfoundry.org/tlsconfig"
	"code.cloudfoundry.org/tlsconfig/certtest"
	_ "github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/ory/dockertest/v3"
	"gopkg.in/yaml.v3"

	"github.com/cloudfoundry/streaming-mysql-backup-tool/config"
)

var _ = Describe("streaming-mysql-backup-tool", func() {

	var (
		pool      *dockertest.Pool
		container *dockertest.Resource
		db        *sql.DB
	)

	var (
		session         *gexec.Session
		backupUrl       string
		command         *exec.Cmd
		request         *http.Request
		httpClient      *http.Client
		tmpDir          string
		clientAuthority *certtest.Authority
		serverAuthority *certtest.Authority

		serverCertPEM []byte
		serverKeyPEM  []byte

		backupServerConfig       string
		pidFile                  string
		backupServerPort         int
		clientCA                 string
		enableMutualTLS          bool
		requiredClientIdentities []string
	)

	BeforeEach(func() {
		var err error

		container = nil
		pool, err = dockertest.NewPool("")
		Expect(err).NotTo(HaveOccurred())

		serverAuthority, err = certtest.BuildCA("serverCA")
		Expect(err).ToNot(HaveOccurred())

		serverCert, err := serverAuthority.BuildSignedCertificate("serverCert")
		Expect(err).ToNot(HaveOccurred())
		serverCertPEM, serverKeyPEM, err = serverCert.CertificatePEMAndPrivateKey()
		Expect(err).ToNot(HaveOccurred())

		tmpDir, err = os.MkdirTemp("", "backup-tool-tests")
		Expect(err).ToNot(HaveOccurred())

		pidFile = tmpFilePath("pid")
		backupServerPort = 49000 + GinkgoParallelNode()
		enableMutualTLS = false
		requiredClientIdentities = nil
	})

	AfterEach(func() {
		if container != nil {
			Expect(pool.Purge(container)).To(Succeed())

			Eventually(func() error {
				return docker("volume", "remove", "--force", volumeName)
			}, "1m", "1s").Should(Succeed())
		}
	})

	JustBeforeEach(func() {
		configYAML, err := yaml.Marshal(&config.Config{
			BindAddress: fmt.Sprintf("127.0.0.1:%d", backupServerPort),
			XtraBackup: config.XtraBackup{
				DefaultsFile: "/etc/my.cnf",
				TmpDir:       "/tmp",
			},
			PidFile: pidFile,
			Credentials: config.Credentials{
				Username: "username",
				Password: "password",
			},
			TLS: config.TLSConfig{
				ServerCert:               string(serverCertPEM),
				ServerKey:                string(serverKeyPEM),
				EnableMutualTLS:          enableMutualTLS,
				ClientCA:                 clientCA,
				RequiredClientIdentities: requiredClientIdentities,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		backupServerConfig = string(configYAML)
	})

	AfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			return
		}

		if tmpDir != "" {
			err := os.RemoveAll(tmpDir)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("When mTLS is disabled on the server (the default configuration)", func() {
		JustBeforeEach(func() {
			var (
				err error
			)

			backupUrl = fmt.Sprintf("https://127.0.0.1:%d/backup?format=xbstream", backupServerPort)
			request, err = http.NewRequest("GET", backupUrl, nil)
			Expect(err).ToNot(HaveOccurred())
			request.SetBasicAuth("username", "password")

			serverCertPool, err := serverAuthority.CertPool()
			Expect(err).ToNot(HaveOccurred())

			tlsClientConfig, err := tlsconfig.Build().Client(
				tlsconfig.WithAuthority(serverCertPool),
				tlsconfig.WithServerName("localhost"),
			)

			httpClient = &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: tlsClientConfig,
				},
			}

			command = exec.Command(pathToMainBinary, fmt.Sprintf("-config=%s", backupServerConfig))
			session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() error {
				_, err := httpClient.Get(fmt.Sprintf("https://127.0.0.1:%d", backupServerPort))
				return err
			}, "20s").Should(Succeed())
		})

		AfterEach(func() {
			session.Kill()
			session.Wait()
			err := os.Remove(pidFile)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("When the client uses TLS", func() {
			JustBeforeEach(func() {
				// We wait until the server is up and running and responding to requests
				Eventually(func() error {
					_, err := httpClient.Get(fmt.Sprintf("https://127.0.0.1:%d", backupServerPort))
					return err
				}, "20s").Should(Succeed())
			})

			Describe("Writing PID file", func() {
				var (
					pidFilePath string
				)
				BeforeEach(func() {
					pidFilePath = pidFile
				})

				It("Writes its PID file to the location specified ", func() {
					Expect(pidFilePath).To(BeAnExistingFile())
				})

				It("Checks whether the PID file content matches the process ID", func() {
					fileBytes, err := os.ReadFile(pidFile)
					Expect(err).ToNot(HaveOccurred())
					actualPid, err := strconv.Atoi(string(fileBytes))
					Expect(err).ToNot(HaveOccurred())
					Expect(actualPid).To(Equal(command.Process.Pid))
				})
			})

			Describe("Initiating a backup", func() {
				BeforeEach(func() {
					var err error
					container, err = startMySQLServer(pool)
					Expect(err).NotTo(HaveOccurred())

					db, err = containerDB(container)
					Expect(err).NotTo(HaveOccurred())

					Expect(waitForMySQLServer(pool, db)).To(Succeed())
				})

				It("Returns status 200 when the backup is started", func() {
					resp, err := httpClient.Do(request)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(200))
					_, _ = io.Copy(io.Discard, resp.Body)
				})

				It("Returns xbstream output as the response body", func() {
					resp, err := httpClient.Do(request)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(xbstreamExtractTo(tmpDir, resp.Body)).To(Succeed())
					Expect(filepath.Join(tmpDir, "mysql.ibd")).To(BeARegularFile())
				})

				It("Has a trailer with empty Error field if it succeeded", func() {
					resp, err := httpClient.Do(request)
					Expect(err).ShouldNot(HaveOccurred())

					_, err = io.ReadAll(resp.Body)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(resp.Trailer.Get(http.CanonicalHeaderKey("X-Backup-Error"))).To(BeEmpty())
				})

				Context("when the backup is unsuccessful", func() {
					BeforeEach(func() {
						_, err := db.Exec(`REVOKE BACKUP_ADMIN ON *.* FROM root@localhost`)
						Expect(err).NotTo(HaveOccurred())
					})

					It("has HTTP 200 status code but writes the error to the trailer", func() {
						resp, err := httpClient.Do(request)
						Expect(err).ShouldNot(HaveOccurred())

						Expect(resp.StatusCode).To(Equal(200))

						// NOTE: You must read the body from the response in order to populate the response's
						// trailers
						_, _ = io.Copy(io.Discard, resp.Body)

						Expect(resp.Trailer.Get(http.CanonicalHeaderKey("X-Backup-Error"))).To(ContainSubstring("exit status 1"))
						Expect(session).To(gbytes.Say(`Access denied; you need \(at least one of\) the BACKUP_ADMIN privilege\(s\) for this operation`))
					})
				})

				Context("REGRESSION: Hitting the same endpoint twice", func() {
					It("does not fail", func() {
						resp, err := httpClient.Do(request)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(200))
						_, _ = io.Copy(io.Discard, resp.Body)
						Expect(resp.Body.Close()).To(Succeed())

						resp, err = httpClient.Do(request)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(200))
						_, _ = io.Copy(io.Discard, resp.Body)
						Expect(resp.Body.Close()).To(Succeed())
					})
				})
			})

			Context("Basic auth credentials", func() {
				It("Expects Basic Auth credentials", func() {
					resp, err := httpClient.Get(backupUrl)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
					Expect(resp.Header.Get("WWW-Authenticate")).To(Equal(`Basic realm="Authorization Required"`))

					body, err := io.ReadAll(resp.Body)
					Expect(err).ToNot(HaveOccurred())
					Expect(string(body)).To(ContainSubstring(`Not Authorized`))
				})

				It("Accepts good Basic Auth credentials", func() {
					var err error
					container, err = startMySQLServer(pool)
					Expect(err).NotTo(HaveOccurred())

					db, err = containerDB(container)
					Expect(err).NotTo(HaveOccurred())

					Expect(waitForMySQLServer(pool, db)).To(Succeed())

					req, err := http.NewRequest("GET", backupUrl, nil)
					Expect(err).ToNot(HaveOccurred())
					req.SetBasicAuth("username", "password")
					resp, err := httpClient.Do(req)

					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
					_, _ = io.Copy(io.Discard, resp.Body)
					Expect(resp.Body.Close()).To(Succeed())
				})

				It("Does not accept bad Basic Auth credentials", func() {
					req, err := http.NewRequest("GET", backupUrl, nil)
					Expect(err).ToNot(HaveOccurred())
					req.SetBasicAuth("bad_username", "bad_password")

					resp, err := httpClient.Do(req)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

					body, err := io.ReadAll(resp.Body)
					Expect(err).ToNot(HaveOccurred())
					Expect(string(body)).To(ContainSubstring(`Not Authorized`))
				})
			})
		})

		Context("When the client attempts to connect with http URL scheme", func() {
			It("Fails to connect, returning a protocol error", func() {
				backupUrl = fmt.Sprintf("http://127.0.0.1:%d/backup?format=xbstream", backupServerPort)

				httpClient = &http.Client{}
				res, err := httpClient.Get(backupUrl)
				Expect(err).NotTo(HaveOccurred())
				Expect(res.StatusCode).To(Equal(http.StatusBadRequest))
				body, _ := io.ReadAll(res.Body)
				Expect(string(body)).To(ContainSubstring(`Client sent an HTTP request to an HTTPS server.`))
			})
		})

		Context("When the client does not trust the server certificate", func() {
			It("Fails to connect, returning certificate error", func() {
				httpClient = &http.Client{}
				Eventually(func() error {
					_, err := httpClient.Get(backupUrl)
					return err
				}).Should(SatisfyAny(
					MatchError(ContainSubstring("certificate is using a broken key size")),
					MatchError(ContainSubstring("x509: certificate signed by unknown authority")),
				))
			})
		})
	})

	Context("When mTLS is enabled on the server", func() {
		BeforeEach(func() {
			var err error
			clientAuthority, err = certtest.BuildCA("clientCA")
			Expect(err).ToNot(HaveOccurred())

			clientCABytes, err := clientAuthority.CertificatePEM()
			Expect(err).ToNot(HaveOccurred())

			enableMutualTLS = true
			clientCA = string(clientCABytes)
		})

		When("there is a problem with the client's certificate, such as", func() {
			JustBeforeEach(func() {
				backupUrl = fmt.Sprintf("https://127.0.0.1:%d/backup", backupServerPort)
			})

			AfterEach(func() {
				session.Kill()
				session.Wait()
				err := os.Remove(pidFile)
				Expect(err).ToNot(HaveOccurred())
			})

			When("the client does not provide a certificate", func() {
				JustBeforeEach(func() {
					serverCertPool, err := serverAuthority.CertPool()
					Expect(err).ToNot(HaveOccurred())

					TLSClientConfig, err := tlsconfig.Build().Client(
						tlsconfig.WithAuthority(serverCertPool),
						tlsconfig.WithServerName("localhost"),
					)

					httpClient = &http.Client{
						Transport: &http.Transport{
							TLSClientConfig: TLSClientConfig,
						},
					}
					command = exec.Command(pathToMainBinary, fmt.Sprintf("-config=%s", backupServerConfig))
					session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
					Expect(err).ShouldNot(HaveOccurred())

					Eventually(func() error {
						conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", backupServerPort))
						if err == nil {
							conn.Close()
						}
						return err
					}, "20s").Should(Succeed())
				})

				It("Throws a bad certificate error", func() {
					Eventually(func() error {
						_, err := httpClient.Get(backupUrl)
						return err
					}).Should(MatchError(ContainSubstring("tls: bad certificate")))
				})
			})

			When("the client provides a certificate the server does not trust", func() {
				JustBeforeEach(func() {
					clientCertConfig, err := serverAuthority.BuildSignedCertificate("untrusted-client-cert")
					Expect(err).NotTo(HaveOccurred())

					incorrectServerIdentity, err := clientCertConfig.TLSCertificate()
					Expect(err).ToNot(HaveOccurred())

					serverCertPool, err := serverAuthority.CertPool()
					Expect(err).ToNot(HaveOccurred())

					TLSClientConfig, err := tlsconfig.Build(
						tlsconfig.WithIdentity(incorrectServerIdentity), //This is an intentionally invalid identity for the server
					).Client(
						tlsconfig.WithAuthority(serverCertPool),
						tlsconfig.WithServerName("localhost"),
					)

					httpClient = &http.Client{
						Transport: &http.Transport{
							TLSClientConfig: TLSClientConfig,
						},
					}
					command = exec.Command(pathToMainBinary, fmt.Sprintf("-config=%s", backupServerConfig))
					session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
					Expect(err).ShouldNot(HaveOccurred())
				})

				It("Throws a bad certificate error", func() {
					Eventually(func() error {
						_, err := httpClient.Get(backupUrl)
						return err
					}).Should(MatchError(ContainSubstring("tls: bad certificate")))
				})
			})

			When("the client provides a trusted certificate with an invalid identity", func() {
				var backupServerStderrLogs *gbytes.Buffer

				BeforeEach(func() {
					backupServerStderrLogs = gbytes.NewBuffer()
					requiredClientIdentities = []string{"clientCert"}
				})

				JustBeforeEach(func() {
					clientCertConfig, err := clientAuthority.BuildSignedCertificate("invalid-identity", certtest.WithDomains("invalid-identity"))
					Expect(err).ToNot(HaveOccurred())

					backupUrl = fmt.Sprintf("https://127.0.0.1:%d/backup", backupServerPort)
					request, err = http.NewRequest("GET", backupUrl, nil)
					Expect(err).ToNot(HaveOccurred())
					request.SetBasicAuth("username", "password")

					clientIdentity, err := clientCertConfig.TLSCertificate()
					Expect(err).ToNot(HaveOccurred())

					serverCertPool, err := serverAuthority.CertPool()
					Expect(err).ToNot(HaveOccurred())

					mTLSClientConfig, err := tlsconfig.Build(
						tlsconfig.WithIdentity(clientIdentity),
					).Client(
						tlsconfig.WithAuthority(serverCertPool),
						tlsconfig.WithServerName("localhost"),
					)

					httpClient = &http.Client{
						Transport: &http.Transport{
							TLSClientConfig: mTLSClientConfig,
						},
					}

					command = exec.Command(pathToMainBinary, fmt.Sprintf("-config=%s", backupServerConfig))
					session, err = gexec.Start(command, GinkgoWriter, io.MultiWriter(GinkgoWriter, backupServerStderrLogs))
					Expect(err).ShouldNot(HaveOccurred())
				})

				It("Throws a bad certificate error", func() {
					Eventually(func() error {
						_, err := httpClient.Get(backupUrl)
						return err
					}, "5s").Should(MatchError(ContainSubstring("tls: bad certificate")))

					Eventually(backupServerStderrLogs, "1m").
						Should(gbytes.Say(`invalid client identity in presented client certificate`))
				})
			})
		})

		When("TLS options are configured correctly", func() {
			BeforeEach(func() {
				requiredClientIdentities = []string{"clientCert"}
			})

			JustBeforeEach(func() {
				clientCertConfig, err := clientAuthority.BuildSignedCertificate("clientCert", certtest.WithDomains("clientCert"))
				Expect(err).ToNot(HaveOccurred())

				backupUrl = fmt.Sprintf("https://127.0.0.1:%d/backup?format=xbstream", backupServerPort)
				request, err = http.NewRequest("GET", backupUrl, nil)
				Expect(err).ToNot(HaveOccurred())

				clientIdentity, err := clientCertConfig.TLSCertificate()
				Expect(err).ToNot(HaveOccurred())

				serverCertPool, err := serverAuthority.CertPool()
				Expect(err).ToNot(HaveOccurred())

				mTLSClientConfig, err := tlsconfig.Build(
					tlsconfig.WithIdentity(clientIdentity),
				).Client(
					tlsconfig.WithAuthority(serverCertPool),
					tlsconfig.WithServerName("localhost"),
				)

				httpClient = &http.Client{
					Transport: &http.Transport{
						TLSClientConfig: mTLSClientConfig,
					},
				}
				command = exec.Command(pathToMainBinary, fmt.Sprintf("-config=%s", backupServerConfig))
				session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
				Expect(err).ShouldNot(HaveOccurred())
				Eventually(func() error {
					_, err := httpClient.Get(fmt.Sprintf("https://127.0.0.1:%d", backupServerPort))
					return err
				}, "20s").Should(Succeed())
			})

			AfterEach(func() {
				session.Kill()
				session.Wait()
				err := os.Remove(pidFile)
				Expect(err).ToNot(HaveOccurred())
			})

			Context("When the client uses TLS", func() {
				JustBeforeEach(func() {
					// We wait until the server is up and running and responding to requests
					Eventually(func() error {
						_, err := httpClient.Get(fmt.Sprintf("https://127.0.0.1:%d", backupServerPort))
						return err
					}, "20s").Should(Succeed())
				})

				Describe("Writing PID file", func() {
					var (
						pidFilePath string
					)
					BeforeEach(func() {
						pidFilePath = pidFile
					})

					It("Writes its PID file to the location specified ", func() {
						Expect(pidFilePath).To(BeAnExistingFile())
					})

					It("Checks whether the PID file content matches the process ID", func() {
						fileBytes, err := os.ReadFile(pidFile)
						Expect(err).ToNot(HaveOccurred())
						actualPid, err := strconv.Atoi(string(fileBytes))
						Expect(err).ToNot(HaveOccurred())
						Expect(actualPid).To(Equal(command.Process.Pid))
					})
				})

				Describe("Initiating a backup", func() {
					BeforeEach(func() {
						var err error
						container, err = startMySQLServer(pool)
						Expect(err).NotTo(HaveOccurred())

						db, err = containerDB(container)
						Expect(err).NotTo(HaveOccurred())

						Expect(waitForMySQLServer(pool, db)).To(Succeed())
					})

					It("Returns status 200 when the backup is scheduled", func() {
						resp, err := httpClient.Do(request)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(200))
						_, _ = io.Copy(io.Discard, resp.Body)
						_ = resp.Body.Close()
					})

					It("Returns the output from the configured backup command as the response body", func() {
						resp, err := httpClient.Do(request)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(xbstreamExtractTo(tmpDir, resp.Body)).To(Succeed())
						Expect(filepath.Join(tmpDir, "mysql.ibd")).To(BeARegularFile())
					})

					It("Has a trailer with empty Error field if it succeeded", func() {
						resp, err := httpClient.Do(request)
						Expect(err).ShouldNot(HaveOccurred())

						_, _ = io.Copy(io.Discard, resp.Body)

						Expect(resp.Trailer.Get(http.CanonicalHeaderKey("X-Backup-Error"))).To(BeEmpty())
					})

					Context("when the backup is unsuccessful", func() {
						BeforeEach(func() {
							_, err := db.Exec(`REVOKE BACKUP_ADMIN ON *.* FROM root@localhost`)
							Expect(err).NotTo(HaveOccurred())
						})

						It("has HTTP 200 status code but writes the error to the trailer", func() {
							resp, err := httpClient.Do(request)
							Expect(err).ShouldNot(HaveOccurred())

							Expect(resp.StatusCode).To(Equal(200))

							// NOTE: You must read the body from the response in order to populate the response's
							// trailers
							_, _ = io.Copy(io.Discard, resp.Body)

							Expect(resp.Trailer.Get(http.CanonicalHeaderKey("X-Backup-Error"))).To(ContainSubstring("exit status 1"))
						})
					})
				})

				Describe("REGRESSION: Hitting the same endpoint twice", func() {
					BeforeEach(func() {
						var err error
						container, err = startMySQLServer(pool)
						Expect(err).NotTo(HaveOccurred())

						db, err = containerDB(container)
						Expect(err).NotTo(HaveOccurred())

						Expect(waitForMySQLServer(pool, db)).To(Succeed())
					})

					It("does not fail", func() {
						resp, err := httpClient.Do(request)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(200))
						_, _ = io.Copy(io.Discard, resp.Body)
						_ = resp.Body.Close()

						resp, err = httpClient.Do(request)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(200))
						_, _ = io.Copy(io.Discard, resp.Body)
						_ = resp.Body.Close()
					})
				})

				Describe("Basic auth credentials", func() {
					BeforeEach(func() {
						var err error
						container, err = startMySQLServer(pool)
						Expect(err).NotTo(HaveOccurred())

						db, err = containerDB(container)
						Expect(err).NotTo(HaveOccurred())

						Expect(waitForMySQLServer(pool, db)).To(Succeed())
					})

					It("Accepts bad Basic Auth credentials", func() {
						req, err := http.NewRequest("GET", backupUrl, nil)
						Expect(err).ToNot(HaveOccurred())
						req.SetBasicAuth("bad_username", "bad_password")
						resp, err := httpClient.Do(req)

						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusOK))
						_, _ = io.Copy(io.Discard, resp.Body)
						Expect(resp.Body.Close()).To(Succeed())
					})
				})
			})

			Context("When the client attempts to connect with http URL scheme", func() {
				It("Fails to connect, returning a protocol error", func() {
					backupUrl = fmt.Sprintf("http://127.0.0.1:%d/backup?format=xbstream", backupServerPort)
					httpClient = &http.Client{}
					res, err := httpClient.Get(backupUrl)
					Expect(err).NotTo(HaveOccurred())
					Expect(res.StatusCode).To(Equal(http.StatusBadRequest))
					body, _ := io.ReadAll(res.Body)
					Expect(string(body)).To(ContainSubstring("Client sent an HTTP request to an HTTPS server"))
				})
			})

			Context("When the client does not trust the server certificate", func() {
				It("Fails to connect, returning certificate error", func() {
					httpClient = &http.Client{}
					Eventually(func() error {
						_, err := httpClient.Get(backupUrl)
						return err
					}).Should(SatisfyAny(
						MatchError(ContainSubstring("certificate is using a broken key size")),
						MatchError(ContainSubstring("x509: certificate signed by unknown authority")),
					))
				})
			})
		})
	})
})

func containerDB(container *dockertest.Resource) (*sql.DB, error) {
	return sql.Open("mysql", "root@tcp(127.0.0.1:"+container.GetPort("3306/tcp")+")/")
}

func startMySQLServer(pool *dockertest.Pool) (*dockertest.Resource, error) {
	return pool.RunWithOptions(&dockertest.RunOptions{
		Name:         "mysql." + sessionID,
		Repository:   "percona/percona-server",
		Tag:          "8.0",
		Env:          []string{"MYSQL_ALLOW_EMPTY_PASSWORD=1"},
		ExposedPorts: []string{"3306/tcp"},
		Mounts:       []string{volumeName + ":/var/lib/mysql"},
	})
}

func waitForMySQLServer(pool *dockertest.Pool, db *sql.DB) error {
	return pool.Retry(db.Ping)
}

func xbstreamExtractTo(path string, input io.Reader) error {
	cmd := exec.Command("xbstream", "-C", path, "-x")
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	cmd.Stdin = input
	return cmd.Run()
}
