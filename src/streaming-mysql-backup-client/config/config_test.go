package config_test

import (
	"code.cloudfoundry.org/tlsconfig/certtest"
	"fmt"
	configPkg "github.com/cloudfoundry/streaming-mysql-backup-client/config"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ClientConfig", func() {
	var (
		configuration string

		serverName      string
		serverCA        string
		clientCert      string
		clientKey       string
		enableMutualTLS bool
		osArgs          []string
	)

	BeforeEach(func() {
		serverName = "myServerName"

		ca, err := certtest.BuildCA("serverCA")
		Expect(err).ToNot(HaveOccurred())
		caBytes, err := ca.CertificatePEM()
		Expect(err).ToNot(HaveOccurred())

		serverCA = string(caBytes)

		certificate, err := ca.BuildSignedCertificate("clientCert")
		Expect(err).ToNot(HaveOccurred())
		certPEM, privateKey, err := certificate.CertificatePEMAndPrivateKey()
		Expect(err).ToNot(HaveOccurred())

		clientCert = string(certPEM)
		clientKey = string(privateKey)
	})

	JustBeforeEach(func() {
		configurationTemplate := `{
						"Ips": ["fakeIp"],
						"BackupServerPort": 8081,
						"BackupAllMasters": false,
						"BackupFromInactiveNode": false,
						"GaleraAgentPort": null,
						"Credentials":{
							"Username": "fake_username",
							"Password": "fake_password",
						},
						"TLS": {
							"EnableMutualTLS": %t,
							"ClientCert": %q,
							"ClientKey": %q,
							"ServerName": %q,
							"ServerCACert": %q,
						},
						"TmpDir": "fakeTmp",
						"OutputDir": "fakeOutput",
						"SymmetricKey": "fakeKey",
					}`

		configuration = fmt.Sprintf(
			configurationTemplate, enableMutualTLS, clientCert, clientKey, serverName, serverCA,
		)

		osArgs = []string{
			"streaming-mysql-backup-client",
			fmt.Sprintf("-config=%s", configuration),
		}
	})

	It("Creates a TlsConfig", func() {
		rootConfig, err := configPkg.NewConfig(osArgs)
		Expect(err).NotTo(HaveOccurred())

		Expect(rootConfig.TLS.Config).NotTo(BeNil())
		Expect(rootConfig.TLS.Config.RootCAs).NotTo(BeNil())
		Expect(rootConfig.TLS.Config.ServerName).To(Equal("myServerName"))
		Expect(rootConfig.TLS.Config.Certificates).To(HaveLen(0)) // mTLS is off by default
		Expect(rootConfig.TLS.Config.CipherSuites).NotTo(BeEmpty())
	})

	Context("When server CA certificate does not exist", func() {
		BeforeEach(func() {
			serverCA = "invalid_ca"
		})

		It("Returns an error", func() {
			_, err := configPkg.NewConfig(osArgs)
			Expect(err).To(MatchError("unable to load server CA certificate"))
		})
	})

	Context("When server CA is an invalid certificate", func() {
		BeforeEach(func() {
			serverCA = clientKey
		})

		It("Returns an error", func() {
			_, err := configPkg.NewConfig(osArgs)
			Expect(err).To(MatchError("unable to load server CA certificate"))
		})
	})

	Context("When Mutual TLS is enabled", func() {
		BeforeEach(func() {
			enableMutualTLS = true
		})

		It("Creates a TlsConfig", func() {
			rootConfig, err := configPkg.NewConfig(osArgs)
			Expect(err).NotTo(HaveOccurred())

			Expect(rootConfig.TLS.Config).NotTo(BeNil())
			Expect(rootConfig.TLS.Config.RootCAs).NotTo(BeNil())
			Expect(rootConfig.TLS.Config.ServerName).To(Equal("myServerName"))
			Expect(rootConfig.TLS.Config.Certificates).To(HaveLen(1))
		})

		Context("When client certificate file does not exist", func() {
			BeforeEach(func() {
				clientCert = "invalid_cert"
			})

			It("Returns an error", func() {
				_, err := configPkg.NewConfig(osArgs)
				Expect(err).To(MatchError("failed to load client certificate or private key: tls: failed to find any PEM data in certificate input"))
			})
		})

		Context("When client certificate is an invalid certificate", func() {
			BeforeEach(func() {
				clientCert = clientKey
			})

			It("Returns an error", func() {
				_, err := configPkg.NewConfig(osArgs)
				Expect(err).To(MatchError("failed to load client certificate or private key: tls: failed to find certificate PEM data in certificate input, but did find a private key; PEM inputs may have been switched"))
			})
		})

	})
})
