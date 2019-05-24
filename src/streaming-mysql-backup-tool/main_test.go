package main_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"code.cloudfoundry.org/tlsconfig"
	"code.cloudfoundry.org/tlsconfig/certtest"

	"streaming-mysql-backup-tool/config"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
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
		rootConfig           config.Config
		expectedResponseBody = "my_output"

		tmpDir     string
		clientCA   *certtest.Authority
		clientCert *certtest.Certificate
		serverCA   *certtest.Authority
		serverCert *certtest.Certificate
	)

	BeforeEach(func() {
		var err error
		clientCA, err = certtest.BuildCA("clientCA")
		Expect(err).ToNot(HaveOccurred())
		clientCABytes, err := clientCA.CertificatePEM()
		Expect(err).ToNot(HaveOccurred())

		serverCA, err = certtest.BuildCA("serverCA")
		Expect(err).ToNot(HaveOccurred())

		serverCert, err = serverCA.BuildSignedCertificate("serverCert")
		Expect(err).ToNot(HaveOccurred())
		serverCertPEM, privateServerKey, err := serverCert.CertificatePEMAndPrivateKey()
		Expect(err).ToNot(HaveOccurred())

		tmpDir, err = ioutil.TempDir("", "backup-tool-tests")
		Expect(err).ToNot(HaveOccurred())

		clientCAPath := filepath.Join(tmpDir, "clientCA.crt")
		Expect(ioutil.WriteFile(clientCAPath, clientCABytes, 0666)).To(Succeed())
		serverCertPath := filepath.Join(tmpDir, "server.crt")
		Expect(ioutil.WriteFile(serverCertPath, serverCertPEM, 0666)).To(Succeed())
		serverKeyPath := filepath.Join(tmpDir, "server.key")
		Expect(ioutil.WriteFile(serverKeyPath, privateServerKey, 0666)).To(Succeed())

		rootConfig = config.Config{
			Port:    int(49000 + GinkgoParallelNode()),
			Command: fmt.Sprintf("echo -n %s", expectedResponseBody),
			PidFile: tmpFilePath("pid"),
			Credentials: config.Credentials{
				Username: "username",
				Password: "password",
			},
			Certificates: config.Certificates{
				Cert:     serverCertPath,
				Key:      serverKeyPath,
				ClientCA: clientCAPath,
			},
		}
	})

	AfterEach(func() {
		if tmpDir != "" {
			err := os.RemoveAll(tmpDir)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("When mTLS is disabled on the server (the default configuration)", func() {
		JustBeforeEach(func() {
			// In case individual tests want to modify their rootConfig variable after BeforeEach
			writeConfig(rootConfig)

			var (
				err error
			)

			backupUrl = fmt.Sprintf("https://localhost:%d/backup", rootConfig.Port)
			request, err = http.NewRequest("GET", backupUrl, nil)
			Expect(err).ToNot(HaveOccurred())
			request.SetBasicAuth("username", "password")

			serverCertPool, err := serverCA.CertPool()
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

			command = exec.Command(pathToMainBinary, fmt.Sprintf("-configPath=%s", configPath))
			session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			session.Kill()
			session.Wait()
			err := os.Remove(rootConfig.PidFile)
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
					pidFilePath = rootConfig.PidFile
				})

				It("Writes its PID file to the location specified ", func() {
					Expect(pidFilePath).To(BeAnExistingFile())
				})

				It("Checks whether the PID file content matches the process ID", func() {
					fileBytes, err := ioutil.ReadFile(rootConfig.PidFile)
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
						rootConfig.Command = "cat nonexistentfile"
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

						rootConfig.Command = saveBashScript(longRunningScript)
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
				backupUrl = fmt.Sprintf("http://localhost:%d/backup", rootConfig.Port)

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
			rootConfig.EnableMutualTLS = true
		})

		Context("When there is a problem with the client's certificate, such as", func() {
			JustBeforeEach(func() {
				// In case individual tests want to modify their rootConfig variable after BeforeEach
				writeConfig(rootConfig)

				backupUrl = fmt.Sprintf("https://localhost:%d/backup", rootConfig.Port)
			})

			AfterEach(func() {
				session.Kill()
				session.Wait()
				err := os.Remove(rootConfig.PidFile)
				Expect(err).ToNot(HaveOccurred())
			})

			Context("When the client does not provide a certificate", func() {
				JustBeforeEach(func() {
					serverCertPool, err := serverCA.CertPool()
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
					command = exec.Command(pathToMainBinary, fmt.Sprintf("-configPath=%s", configPath))
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

			Context("When the client provides a certificate the server does not trust", func() {
				JustBeforeEach(func() {
					incorrectServerIdentity, err := serverCert.TLSCertificate()
					Expect(err).ToNot(HaveOccurred())

					serverCertPool, err := serverCA.CertPool()
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
					command = exec.Command(pathToMainBinary, fmt.Sprintf("-configPath=%s", configPath))
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
		})

		Context("When TLS options are configured correctly", func() {
			JustBeforeEach(func() {
				// In case individual tests want to modify their rootConfig variable after BeforeEach
				writeConfig(rootConfig)

				var err error
				clientCert, err = clientCA.BuildSignedCertificate("clientCert")
				Expect(err).ToNot(HaveOccurred())

				backupUrl = fmt.Sprintf("https://localhost:%d/backup", rootConfig.Port)
				request, err = http.NewRequest("GET", backupUrl, nil)
				Expect(err).ToNot(HaveOccurred())
				request.SetBasicAuth("username", "password")

				clientIdentity, err := clientCert.TLSCertificate()
				Expect(err).ToNot(HaveOccurred())

				serverCertPool, err := serverCA.CertPool()
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
				command = exec.Command(pathToMainBinary, fmt.Sprintf("-configPath=%s", configPath))
				session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
				Expect(err).ShouldNot(HaveOccurred())
			})

			AfterEach(func() {
				session.Kill()
				session.Wait()
				err := os.Remove(rootConfig.PidFile)
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
						pidFilePath = rootConfig.PidFile
					})

					It("Writes its PID file to the location specified ", func() {
						Expect(pidFilePath).To(BeAnExistingFile())
					})

					It("Checks whether the PID file content matches the process ID", func() {
						fileBytes, err := ioutil.ReadFile(rootConfig.PidFile)
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
							rootConfig.Command = "cat nonexistentfile"
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

							rootConfig.Command = saveBashScript(longRunningScript)
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
					backupUrl = fmt.Sprintf("http://localhost:%d/backup", rootConfig.Port)

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
