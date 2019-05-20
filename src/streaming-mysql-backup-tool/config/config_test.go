package config_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	configPkg "streaming-mysql-backup-tool/config"

	"code.cloudfoundry.org/tlsconfig/certtest"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-cf-experimental/service-config/test_helpers"
	"streaming-mysql-backup-tool/config"
)

var _ = Describe("Config", func() {

	var (
		rootConfig    *config.Config
		configuration string
	)

	JustBeforeEach(func() {
		osArgs := []string{
			"streaming-mysql-backup-tool",
			fmt.Sprintf("-config=%s", configuration),
		}

		var err error
		rootConfig, err = config.NewConfig(osArgs)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Validate", func() {
		BeforeEach(func() {
			configuration = `{
				"Command": fakeCommand,
				"Port": 8081,
				"PidFile": fakePath,
				"Credentials":{
					"Username": "fake_username",
					"Password": "fake_password",
				},
				"Certificates":{
					"Cert": "cert_path",
					"Key": "key_path",
					"ClientCA": "CA_path",
				}
			}`
		})

		It("does not return error on valid config", func() {
			err := rootConfig.Validate()
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error if pidfile is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "PidFile")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Command is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Command")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Port is zero", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Port")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Credentials is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Credentials")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Credentials.Username is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Credentials.Username")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Credentials.Password is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Credentials.Password")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Certificates is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Certificates")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Certificates.Cert is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Certificates.Cert")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Certificates.Key is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Certificates.Key")
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error if Certificates.ClientCA is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Certificates.ClientCA")
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Describe("Cmd", func() {
		BeforeEach(func() {
			configuration = `{
				"Command": "echo -n hello"
			}`
		})

		It("parses the command option", func() {
			cmd := rootConfig.Cmd()

			Expect(cmd.Args).To(Equal([]string{"echo", "-n", "hello"}))
		})
	})

	Describe("CreateTlsConfig", func() {
		var (
			rootConfig    *configPkg.Config
			configuration string

			caPath         string
			clientCaPath   string
			certPath       string
			privateKeyPath string
			tmpDir         string
		)
		BeforeEach(func() {
			ca, err := certtest.BuildCA("serverCA")
			Expect(err).ToNot(HaveOccurred())
			caBytes, err := ca.CertificatePEM()
			Expect(err).ToNot(HaveOccurred())

			certificate, err := ca.BuildSignedCertificate("serverCert")
			Expect(err).ToNot(HaveOccurred())
			certPEM, privateKey, err := certificate.CertificatePEMAndPrivateKey()
			Expect(err).ToNot(HaveOccurred())

			clientCa, err := certtest.BuildCA("clientCA")
			Expect(err).ToNot(HaveOccurred())
			clientCaBytes, err := clientCa.CertificatePEM()
			Expect(err).ToNot(HaveOccurred())

			tmpDir, err = ioutil.TempDir("", "backup-tool-tests")
			Expect(err).ToNot(HaveOccurred())

			caPath = filepath.Join(tmpDir, "ca.crt")
			Expect(ioutil.WriteFile(caPath, caBytes, 0666)).To(Succeed())
			certPath = filepath.Join(tmpDir, "cert.crt")
			Expect(ioutil.WriteFile(certPath, certPEM, 0666)).To(Succeed())
			privateKeyPath = filepath.Join(tmpDir, "key.key")
			Expect(ioutil.WriteFile(privateKeyPath, privateKey, 0666)).To(Succeed())
			clientCaPath = filepath.Join(tmpDir, "client-ca.crt")
			Expect(ioutil.WriteFile(clientCaPath, clientCaBytes, 0666)).To(Succeed())
		})

		JustBeforeEach(func() {
			osArgs := []string{
				"streaming-mysql-backup-client",
				fmt.Sprintf("-config=%s", configuration),
			}

			var err error
			rootConfig, err = configPkg.NewConfig(osArgs)
			Expect(err).ToNot(HaveOccurred())
		})

		Context("When Mutual TLS is enabled", func() {
			BeforeEach(func() {
				configuration = fmt.Sprintf(`{
				"Certificates": {
					"Cert": %q,
					"Key": %q,
					"ClientCA": %q
				},
                "EnableMutualTLS": true
			}`, certPath, privateKeyPath, clientCaPath)
			})

			Context("When certificates are valid", func() {
				It("Creates a TlsConfig suitable for mTLS", func() {
					Expect(rootConfig.CreateTlsConfig()).To(Succeed())
					Expect(rootConfig.Certificates.TlsConfig.Certificates).To(HaveLen(1))
					Expect(rootConfig.Certificates.TlsConfig.ClientCAs.Subjects()).To(HaveLen(1))
				})
			})

			Context("When certificates are invalid", func() {
				Context("When certificate file does not exist", func() {
					It("Returns an error", func() {
						rootConfig.Certificates.ClientCA = "invalid_path"
						err := rootConfig.CreateTlsConfig()
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError("failed to read file invalid_path: open invalid_path: no such file or directory"))
					})
				})

				Context("When CA file is an invalid certificate", func() {
					It("Returns an error", func() {
						rootConfig.Certificates.ClientCA = privateKeyPath
						err := rootConfig.CreateTlsConfig()
						Expect(err).To(HaveOccurred())
						Expect(err).To(MatchError("unable to load CA certificate at " + privateKeyPath))
					})
				})
			})
		})

		Context("When Mutual TLS is not enabled", func() {
			BeforeEach(func() {
				configuration = fmt.Sprintf(`{
				"Certificates": {
					"Cert": %q,
					"Key": %q,
					"ClientCA": ""
				}
                }`, certPath, privateKeyPath)
			})

			It("Creates a TlsConfig without a Client CA", func() {
				Expect(rootConfig.CreateTlsConfig()).To(Succeed())
				Expect(rootConfig.Certificates.TlsConfig.Certificates).To(HaveLen(1))
				Expect(rootConfig.Certificates.TlsConfig.ClientCAs).To(BeNil())
			})
		})
	})

})
