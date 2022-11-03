package main_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"code.cloudfoundry.org/tlsconfig"
	"code.cloudfoundry.org/tlsconfig/certtest"
	"github.com/onsi/gomega/gbytes"
	"gopkg.in/yaml.v2"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"github.com/cloudfoundry/streaming-mysql-backup-tool/config"
)

func saveScript(scriptContents string) string {
	tmpFile := createTmpFile("testScript")

	_, err := tmpFile.WriteString(scriptContents)
	Expect(err).ShouldNot(HaveOccurred())

	err = tmpFile.Close()
	Expect(err).ShouldNot(HaveOccurred())

	filePath, err := filepath.Abs(tmpFile.Name())
	Expect(err).ShouldNot(HaveOccurred())

	return filePath
}

func saveBashScript(scriptContents string) string {
	scriptPath := saveScript("#!/bin/bash\n" + scriptContents)

	return "bash " + scriptPath
}

var _ = Describe("Main", func() {

	var (
		session              *gexec.Session
		backupUrl            string
		command              *exec.Cmd
		request              *http.Request
		httpClient           *http.Client
		expectedResponseBody = "my_output"
		tmpDir               string
		clientAuthority      *certtest.Authority
		serverAuthority      *certtest.Authority

		serverCertPEM []byte
		serverKeyPEM  []byte

		backupServerConfig       string
		pidFile                  string
		backupServerPort         int
		backupServerCmd          string
		clientCA                 string
		enableMutualTLS          bool
		requiredClientIdentities []string
	)

	BeforeEach(func() {
		var err error
		serverAuthority, err = certtest.BuildCA("serverCA")
		Expect(err).ToNot(HaveOccurred())

		serverCert, err := serverAuthority.BuildSignedCertificate("serverCert")
		Expect(err).ToNot(HaveOccurred())
		serverCertPEM, serverKeyPEM, err = serverCert.CertificatePEMAndPrivateKey()
		Expect(err).ToNot(HaveOccurred())

		tmpDir, err = ioutil.TempDir("", "backup-tool-tests")
		Expect(err).ToNot(HaveOccurred())

		pidFile = tmpFilePath("pid")
		backupServerPort = int(49000 + GinkgoParallelNode())
		backupServerCmd = fmt.Sprintf("echo -n %s", expectedResponseBody)
		enableMutualTLS = false
		requiredClientIdentities = nil
	})

	JustBeforeEach(func() {
		configYAML, err := yaml.Marshal(&config.Config{
			Port:    backupServerPort,
			Command: backupServerCmd,
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

			backupUrl = fmt.Sprintf("https://localhost:%d/backup", backupServerPort)
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
					_, err := httpClient.Get(backupUrl)
					return err
				}).Should(Succeed())
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
					fileBytes, err := ioutil.ReadFile(pidFile)
					Expect(err).ToNot(HaveOccurred())
					actualPid, err := strconv.Atoi(string(fileBytes))
					Expect(err).ToNot(HaveOccurred())
					Expect(actualPid).To(Equal(command.Process.Pid))
				})
			})

			Describe("Initiating a backup", func() {
				It("Returns status 200 when the backup is scheduled", func() {
					resp, err := httpClient.Do(request)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(200))
				})

				It("Returns the output from the configured backup command as the response body", func() {
					resp, err := httpClient.Do(request)
					Expect(err).ShouldNot(HaveOccurred())

					body, err := ioutil.ReadAll(resp.Body)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(body).To(Equal([]byte(expectedResponseBody)))
				})

				It("Has a trailer with empty Error field if it succeeded", func() {
					resp, err := httpClient.Do(request)
					Expect(err).ShouldNot(HaveOccurred())

					_, err = ioutil.ReadAll(resp.Body)
					Expect(err).ShouldNot(HaveOccurred())

					Expect(resp.Trailer.Get(http.CanonicalHeaderKey("X-Backup-Error"))).To(BeEmpty())
				})

				Context("when the backup is unsuccessful", func() {
					BeforeEach(func() {
						backupServerCmd = "cat nonexistentfile"
					})

					It("has HTTP 200 status code but writes the error to the trailer", func() {
						resp, err := httpClient.Do(request)
						Expect(err).ShouldNot(HaveOccurred())

						Expect(resp.StatusCode).To(Equal(200))

						// NOTE: You must read the body from the response in order to populate the response's
						// trailers
						body, err := ioutil.ReadAll(resp.Body)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(body).To(Equal([]byte("")))

						Expect(resp.Trailer.Get(http.CanonicalHeaderKey("X-Backup-Error"))).To(ContainSubstring("exit status 1"))
					})
				})

				Context("when the command fails halfway through", func() {
					BeforeEach(func() {
						// https://www.percona.com/doc/percona-xtrabackup/2.1/xtrabackup_bin/xtrabackup_exit_codes.html
						longRunningScript := `echo -n hello
										exit 1
										echo world`

						backupServerCmd = saveBashScript(longRunningScript)
					})

					It("has HTTP 200 status code but writes the error to the trailer", func() {
						resp, err := httpClient.Do(request)
						Expect(err).ShouldNot(HaveOccurred())

						Expect(resp.StatusCode).To(Equal(200))

						// NOTE: You must read the body from the response in order to populate the response's
						// trailers
						body, err := ioutil.ReadAll(resp.Body)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(body).To(Equal([]byte("hello")))

						Expect(resp.Trailer.Get(http.CanonicalHeaderKey("X-Backup-Error"))).To(ContainSubstring("exit status 1"))
					})
				})
			})

			Describe("REGRESSION: Hitting the same endpoint twice", func() {
				It("does not fail", func() {
					resp, err := httpClient.Do(request)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(200))

					resp, err = httpClient.Do(request)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(200))
				})
			})

			Describe("Basic auth credentials", func() {
				It("Expects Basic Auth credentials", func() {
					resp, err := httpClient.Get(backupUrl)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
					Expect(resp.Header.Get("WWW-Authenticate")).To(Equal(`Basic realm="Authorization Required"`))

					body, err := ioutil.ReadAll(resp.Body)
					Expect(err).ToNot(HaveOccurred())
					Expect(body).ToNot(ContainSubstring(expectedResponseBody))
				})

				It("Accepts good Basic Auth credentials", func() {
					req, err := http.NewRequest("GET", backupUrl, nil)
					Expect(err).ToNot(HaveOccurred())
					req.SetBasicAuth("username", "password")
					resp, err := httpClient.Do(req)

					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusOK))
				})

				It("Does not accept bad Basic Auth credentials", func() {
					req, err := http.NewRequest("GET", backupUrl, nil)
					Expect(err).ToNot(HaveOccurred())
					req.SetBasicAuth("bad_username", "bad_password")

					resp, err := httpClient.Do(req)
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))

					body, err := ioutil.ReadAll(resp.Body)
					Expect(err).ToNot(HaveOccurred())
					Expect(body).ToNot(ContainSubstring(expectedResponseBody))
				})
			})
		})

		Context("When the client attempts to connect with http URL scheme", func() {
			It("Fails to connect, returning a protocol error", func() {
				backupUrl = fmt.Sprintf("http://localhost:%d/backup", backupServerPort)

				httpClient = &http.Client{}
				Eventually(func() bool {
					res, err := httpClient.Get(backupUrl)
					if err != nil {
						if strings.Contains(err.Error(), "malformed HTTP response") {
							return true
						}
					} else {
						// Compatible fix for golang > 1.12, which no longer return an error.
						resbody, err := ioutil.ReadAll(res.Body)
						Expect(err).ToNot(HaveOccurred())
						if res.StatusCode == 400 && strings.Contains(string(resbody), "Client sent an HTTP request to an HTTPS server.") {
							return true
						}
					}
					return false
				}).Should(BeTrue())
			})
		})

		Context("When the client does not trust the server certificate", func() {
			It("Fails to connect, returning certificate error", func() {
				httpClient = &http.Client{}
				Eventually(func() error {
					_, err := httpClient.Get(backupUrl)
					return err
				}).Should(MatchError(ContainSubstring("x509: certificate signed by unknown authority")))
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
				backupUrl = fmt.Sprintf("https://localhost:%d/backup", backupServerPort)
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

					backupUrl = fmt.Sprintf("https://localhost:%d/backup", backupServerPort)
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

				backupUrl = fmt.Sprintf("https://localhost:%d/backup", backupServerPort)
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
						_, err := httpClient.Get(backupUrl)
						return err
					}).Should(Succeed())
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
						fileBytes, err := ioutil.ReadFile(pidFile)
						Expect(err).ToNot(HaveOccurred())
						actualPid, err := strconv.Atoi(string(fileBytes))
						Expect(err).ToNot(HaveOccurred())
						Expect(actualPid).To(Equal(command.Process.Pid))
					})
				})

				Describe("Initiating a backup", func() {
					It("Returns status 200 when the backup is scheduled", func() {
						resp, err := httpClient.Do(request)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(200))
					})

					It("Returns the output from the configured backup command as the response body", func() {
						resp, err := httpClient.Do(request)
						Expect(err).ShouldNot(HaveOccurred())

						body, err := ioutil.ReadAll(resp.Body)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(body).To(Equal([]byte(expectedResponseBody)))
					})

					It("Has a trailer with empty Error field if it succeeded", func() {
						resp, err := httpClient.Do(request)
						Expect(err).ShouldNot(HaveOccurred())

						_, err = ioutil.ReadAll(resp.Body)
						Expect(err).ShouldNot(HaveOccurred())

						Expect(resp.Trailer.Get(http.CanonicalHeaderKey("X-Backup-Error"))).To(BeEmpty())
					})

					Context("when the backup is unsuccessful", func() {
						BeforeEach(func() {
							backupServerCmd = "cat nonexistentfile"
						})

						It("has HTTP 200 status code but writes the error to the trailer", func() {
							resp, err := httpClient.Do(request)
							Expect(err).ShouldNot(HaveOccurred())

							Expect(resp.StatusCode).To(Equal(200))

							// NOTE: You must read the body from the response in order to populate the response's
							// trailers
							body, err := ioutil.ReadAll(resp.Body)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(body).To(Equal([]byte("")))

							Expect(resp.Trailer.Get(http.CanonicalHeaderKey("X-Backup-Error"))).To(ContainSubstring("exit status 1"))
						})
					})

					Context("when the command fails halfway through", func() {
						BeforeEach(func() {
							// https://www.percona.com/doc/percona-xtrabackup/2.1/xtrabackup_bin/xtrabackup_exit_codes.html
							longRunningScript := `echo -n hello
										exit 1
										echo world`

							backupServerCmd = saveBashScript(longRunningScript)
						})

						It("has HTTP 200 status code but writes the error to the trailer", func() {
							resp, err := httpClient.Do(request)
							Expect(err).ShouldNot(HaveOccurred())

							Expect(resp.StatusCode).To(Equal(200))

							// NOTE: You must read the body from the response in order to populate the response's
							// trailers
							body, err := ioutil.ReadAll(resp.Body)
							Expect(err).ShouldNot(HaveOccurred())
							Expect(body).To(Equal([]byte("hello")))

							Expect(resp.Trailer.Get(http.CanonicalHeaderKey("X-Backup-Error"))).To(ContainSubstring("exit status 1"))
						})
					})
				})

				Describe("REGRESSION: Hitting the same endpoint twice", func() {
					It("does not fail", func() {
						resp, err := httpClient.Do(request)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(200))

						resp, err = httpClient.Do(request)
						Expect(err).ShouldNot(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(200))
					})
				})

				Describe("Basic auth credentials", func() {
					It("Accepts bad Basic Auth credentials", func() {
						req, err := http.NewRequest("GET", backupUrl, nil)
						Expect(err).ToNot(HaveOccurred())
						req.SetBasicAuth("bad_username", "bad_password")
						resp, err := httpClient.Do(req)

						Expect(err).NotTo(HaveOccurred())
						Expect(resp.StatusCode).To(Equal(http.StatusOK))
					})
				})
			})

			Context("When the client attempts to connect with http URL scheme", func() {
				It("Fails to connect, returning a protocol error", func() {
					backupUrl = fmt.Sprintf("http://localhost:%d/backup", backupServerPort)

					httpClient = &http.Client{}
					Eventually(func() bool {
						res, err := httpClient.Get(backupUrl)
						if err != nil {
							if strings.Contains(err.Error(), "malformed HTTP response") {
								return true
							}
						} else {
							// Compatible fix for golang > 1.12, which no longer return an error.
							resbody, err := ioutil.ReadAll(res.Body)
							Expect(err).ToNot(HaveOccurred())
							if res.StatusCode == 400 && strings.Contains(string(resbody), "Client sent an HTTP request to an HTTPS server.") {
								return true
							}
						}
						return false
					}).Should(BeTrue())
				})
			})

			Context("When the client does not trust the server certificate", func() {
				It("Fails to connect, returning certificate error", func() {
					httpClient = &http.Client{}
					Eventually(func() error {
						_, err := httpClient.Get(backupUrl)
						return err
					}).Should(MatchError(ContainSubstring("x509: certificate signed by unknown authority")))
				})
			})
		})
	})
})
