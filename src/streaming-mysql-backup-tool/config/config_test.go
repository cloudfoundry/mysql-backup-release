package config_test

import (
	"crypto/tls"
	"fmt"
	"os"

	"code.cloudfoundry.org/tlsconfig/certtest"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry/streaming-mysql-backup-tool/config"
)

var _ = Describe("Config", func() {

	var (
		clientCA        string
		configuration   string
		enableMutualTLS bool
		err             error
		osArgs          []string
		rootConfig      *config.Config
		serverCert      string
		serverKey       string
		tmpDir          string
	)

	BeforeEach(func() {
		// Create certificates
		clientAuthority, err := certtest.BuildCA("clientCA")
		Expect(err).ToNot(HaveOccurred())
		clientCABytes, err := clientAuthority.CertificatePEM()
		Expect(err).ToNot(HaveOccurred())

		serverCA, err := certtest.BuildCA("serverCA")
		Expect(err).ToNot(HaveOccurred())

		serverCertConfig, err := serverCA.BuildSignedCertificate("serverCert")
		Expect(err).ToNot(HaveOccurred())
		serverCertPEM, privateServerKey, err := serverCertConfig.CertificatePEMAndPrivateKey()
		Expect(err).ToNot(HaveOccurred())

		clientCA = string(clientCABytes)
		serverCert = string(serverCertPEM)
		serverKey = string(privateServerKey)
	})

	JustBeforeEach(func() {
		configurationTemplate := `{
				"BindAddress": ":1234",
				"PidFile": fakePath,
				"Credentials":{
					"Username": "fake_username",
					"Password": "fake_password",
				},
				"XtraBackup": {
				  "DefaultsFile": "/etc/my.cnf",
				  "TmpDir": "/tmp",
				},
				"TLS":{
					"ServerCert": %q,
					"ServerKey": %q,
					"ClientCA": %q,
					"EnableMutualTLS": %t
				},
			}`

		configuration = fmt.Sprintf(
			configurationTemplate,
			serverCert,
			serverKey,
			clientCA,
			enableMutualTLS,
		)

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

	It("can load XtraBackup config options", func() {
		rootConfig, err = config.NewConfig(osArgs)
		Expect(err).NotTo(HaveOccurred())

		Expect(rootConfig.XtraBackup.DefaultsFile).To(Equal("/etc/my.cnf"))
		Expect(rootConfig.XtraBackup.TmpDir).To(Equal("/tmp"))
	})

	It("can load a BindAddress option", func() {
		rootConfig, err = config.NewConfig(osArgs)
		Expect(err).NotTo(HaveOccurred())

		Expect(rootConfig.BindAddress).To(Equal(":1234"))
	})

	Context("When TLS Server credentials are misconfigured", func() {
		Context("When server key is invalid", func() {
			BeforeEach(func() {
				serverKey = "invalid-key"
			})

			It("Fails to start with error", func() {
				rootConfig, err = config.NewConfig(osArgs)
				Expect(err).To(MatchError(
					ContainSubstring("failed to load server certificate or private key: tls: failed to find any PEM data in key input"),
				))
			})
		})

		Context("When server certificate is invalid", func() {
			BeforeEach(func() {
				serverCert = "invalid cert"
			})

			It("Fails to start with error", func() {
				rootConfig, err = config.NewConfig(osArgs)
				Expect(err).To(
					MatchError(
						ContainSubstring("failed to load server certificate or private key: tls: failed to find any PEM data in certificate input"),
					),
				)
			})
		})

		// An invalid CA path shouldn't be a problem unless mTLS is enabled
		Context("When client CA path is invalid", func() {
			BeforeEach(func() {
				clientCA = "invalid CA"
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

			Expect(rootConfig.TLS.Config).ToNot(BeNil())
			Expect(rootConfig.TLS.Config.ClientAuth).To(Equal(tls.NoClientCert), "Expected ClientAuth value of tls.NoClientCert")
		})
	})

	Context("When Mutual TLS is enabled", func() {
		BeforeEach(func() {
			enableMutualTLS = true
		})

		It("Requires and verifies client certificates", func() {
			rootConfig, err = config.NewConfig(osArgs)
			Expect(err).ToNot(HaveOccurred())

			Expect(rootConfig.TLS.Config).ToNot(BeNil())
			Expect(rootConfig.TLS.Config.ClientAuth).To(Equal(tls.RequireAndVerifyClientCert), "Expected ClientAuth value of tls.RequireAndVerifyClientCert")
			Expect(rootConfig.TLS.Config.VerifyPeerCertificate).NotTo(BeNil())
		})

		Context("When client CA is invalid", func() {
			BeforeEach(func() {
				clientCA = "invalid CA"
			})

			It("Fails to start with error", func() {
				rootConfig, err = config.NewConfig(osArgs)
				Expect(err).To(
					MatchError(
						ContainSubstring("unable to load client CA certificate"),
					),
				)
			})
		})

	})
})
