package config_test

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
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
	)

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
			command = "fakeCommand"
			serverCertPath = "fakeCertPath"
			serverKeyPath = "fakeKeyPath"
			clientCAPath = "fakeCAPath"
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

		It("parses the command option", func() {
			cmd := rootConfig.Cmd()

			Expect(cmd.Args).To(Equal([]string{"echo", "-n", "hello"}))
		})
	})

	Context("When mTLS is disabled (default behavior)", func() {
		var (
			tmpDir string
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

		It("Does not require or verify client certificates", func() {
			Expect(rootConfig.Certificates.TLSConfig.ClientAuth).To(Equal(tls.NoClientCert), "Expected ClientAuth value of tls.NoClientCert")
		})
	})

	Context("When mTLS is enabled", func() {
		var (
			tmpDir string
		)

		BeforeEach(func() {
			// Enable mTLS
			enableMutualTLS = true

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

		It("Requires and verifies client certificates", func() {
			Expect(rootConfig.Certificates.TLSConfig.ClientAuth).To(Equal(tls.RequireAndVerifyClientCert), "Expected ClientAuth value of tls.RequireAndVerifyClientCert")
		})

	})
})
