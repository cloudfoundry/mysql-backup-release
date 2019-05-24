package config_test

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"code.cloudfoundry.org/tlsconfig/certtest"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-cf-experimental/service-config/test_helpers"

	"streaming-mysql-backup-tool/config"
)

var _ = Describe("Config", func() {

	var (
		rootConfig                                           *config.Config
		configuration                                        string
		command, serverCertPath, serverKeyPath, clientCAPath string
		enableMutualTLS                                      bool
		tmpDir                                               string
		osArgs                                               []string
		err                                                  error
	)

	BeforeEach(func() {
		// Create certificates
		clientCA, err := certtest.BuildCA("clientCA")
		Expect(err).ToNot(HaveOccurred())
		clientCABytes, err := clientCA.CertificatePEM()
		Expect(err).ToNot(HaveOccurred())

		serverCA, err := certtest.BuildCA("serverCA")
		Expect(err).ToNot(HaveOccurred())

		serverCert, err := serverCA.BuildSignedCertificate("serverCert")
		Expect(err).ToNot(HaveOccurred())
		serverCertPEM, privateServerKey, err := serverCert.CertificatePEMAndPrivateKey()
		Expect(err).ToNot(HaveOccurred())

		// Write certificates to files
		tmpDir, err = ioutil.TempDir("", "backup-tool-tests")
		Expect(err).ToNot(HaveOccurred())

		clientCAPath = filepath.Join(tmpDir, "clientCA.crt")
		Expect(ioutil.WriteFile(clientCAPath, clientCABytes, 0666)).To(Succeed())
		serverCertPath = filepath.Join(tmpDir, "server.crt")
		Expect(ioutil.WriteFile(serverCertPath, serverCertPEM, 0666)).To(Succeed())
		serverKeyPath = filepath.Join(tmpDir, "server.key")
		Expect(ioutil.WriteFile(serverKeyPath, privateServerKey, 0666)).To(Succeed())
	})

	JustBeforeEach(func() {
		configurationTemplate := `{
				"Command": %q,
				"Port": 8081,
				"PidFile": fakePath,
				"Credentials":{
					"Username": "fake_username",
					"Password": "fake_password",
				},
				"Certificates":{
					"Cert": %q,
					"Key": %q,
					"ClientCA": %q,
				},
				"EnableMutualTLS": %t
			}`

		configuration = fmt.Sprintf(configurationTemplate, command, serverCertPath, serverKeyPath, clientCAPath, enableMutualTLS)

		osArgs = []string{
			"streaming-mysql-backup-tool",
			fmt.Sprintf("-config=%s", configuration),
		}
	})

	AfterEach(func() {
		if tmpDir != "" {
			err := os.RemoveAll(tmpDir)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Describe("Validate", func() {

		BeforeEach(func() {
			command = "fakeCommand"
		})

		JustBeforeEach(func() {
			rootConfig, err = config.NewConfig(osArgs)
			Expect(err).ToNot(HaveOccurred())
		})

		It("does not return error on valid config", func() {
			err := rootConfig.Validate()
			Expect(err).ToNot(HaveOccurred())
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
			command = "echo -n hello"
		})

		JustBeforeEach(func() {
			rootConfig, err = config.NewConfig(osArgs)
			Expect(err).ToNot(HaveOccurred())
		})

		It("parses the command option", func() {
			cmd := rootConfig.Cmd()
			Expect(cmd.Args).To(Equal([]string{"echo", "-n", "hello"}))
		})
	})

	Context("When TLS Server credentials are misconfigured", func() {
		Context("When server key path is invalid", func() {
			BeforeEach(func() {
				serverKeyPath = "invalidPath"
			})

			It("Fails to start with error", func() {
				rootConfig, err = config.NewConfig(osArgs)
				Expect(err.Error()).To(ContainSubstring("Server key does not exist at location [ invalidPath ]"))
			})
		})

		Context("When server certificate path is invalid", func() {
			BeforeEach(func() {
				serverCertPath = "invalidPath"
			})

			It("Fails to start with error", func() {
				rootConfig, err = config.NewConfig(osArgs)
				Expect(err.Error()).To(ContainSubstring("Server certificate does not exist at location [ invalidPath ]"))
			})
		})

		// An invalid CA path shouldn't be a problem unless mTLS is enabled
		Context("When client CA path is invalid", func() {
			BeforeEach(func() {
				clientCAPath = "invalidPath"
			})

			It("Starts without error", func() {
				rootConfig, err = config.NewConfig(osArgs)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Context("When mTLS is disabled (default behavior)", func() {
		It("Does not require or verify client certificates", func() {
			rootConfig, err = config.NewConfig(osArgs)
			Expect(err).ToNot(HaveOccurred())

			Expect(rootConfig.Certificates.TLSConfig.ClientAuth).To(Equal(tls.NoClientCert), "Expected ClientAuth value of tls.NoClientCert")
		})
	})

	Context("When Mutual TLS is enabled", func() {
		BeforeEach(func() {
			enableMutualTLS = true
		})

		It("Requires and verifies client certificates", func() {
			rootConfig, err = config.NewConfig(osArgs)
			Expect(err).ToNot(HaveOccurred())

			Expect(rootConfig.Certificates.TLSConfig.ClientAuth).To(Equal(tls.RequireAndVerifyClientCert), "Expected ClientAuth value of tls.RequireAndVerifyClientCert")
		})

		Context("When client CA path is invalid", func() {
			BeforeEach(func() {
				clientCAPath = "invalidPath"
			})

			It("Fails to start with error", func() {
				rootConfig, err = config.NewConfig(osArgs)
				Expect(err.Error()).To(ContainSubstring("Client CA certificate does not exist at location [ invalidPath ]"))
			})
		})

	})
})
