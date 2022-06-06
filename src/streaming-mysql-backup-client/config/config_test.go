package config_test

import (
	"fmt"
	"net/http"

	"code.cloudfoundry.org/tlsconfig/certtest"

	configPkg "github.com/cloudfoundry/streaming-mysql-backup-client/config"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("ClientConfig", func() {
	var (
		configuration string

		serverName        string
		serverCA          string
		clientCert        string
		clientKey         string
		enableMutualTLS   bool
		someEncryptionKey string
		osArgs            []string
		galeraAgentCA     string
		galeraAgentName   string
		galeraAgentTLS    bool
	)

	BeforeEach(func() {
		serverName = "myServerName"
		someEncryptionKey = "myEncryptionKey"
		enableMutualTLS = false

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

		caBytes, err = ca.CertificatePEM()
	})

	JustBeforeEach(func() {
		configurationTemplate := `{
						"Instances": [ { "Address": "fakeIp", "UUID": "some-uuid" }],
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
						"BackendTLS": {
							"Enabled": %t,
							"ServerName": %q,
							"CA": %q,
						},
					}`

		configuration = fmt.Sprintf(
			configurationTemplate, enableMutualTLS, clientCert, clientKey, serverName, serverCA,
			galeraAgentTLS, galeraAgentName, galeraAgentCA,
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
		Expect(rootConfig.BackendTLS.Enabled).To(BeFalse())
	})

	When("BackendTLS is enabled", func() {
		BeforeEach(func() {
			galeraAgentTLS = true
			galeraAgentCA = "galeraCA"
			galeraAgentName = "galeraServerName"
		})
		It("Properly populates the BackendTLS", func() {
			rootConfig, err := configPkg.NewConfig(osArgs)
			Expect(err).NotTo(HaveOccurred())
			Expect(rootConfig.BackendTLS.Enabled).To(BeTrue())
			Expect(rootConfig.BackendTLS.CA).To(Equal("galeraCA"))
			Expect(rootConfig.BackendTLS.ServerName).To(Equal("galeraServerName"))
		})
	})

	Context("when the os args omit the encryption key", func() {
		It("Uses the encryption key from the config file", func() {
			rootConfig, err := configPkg.NewConfig(osArgs)
			Expect(err).NotTo(HaveOccurred())

			Expect(rootConfig.SymmetricKey).To(Equal("fakeKey"))
		})
	})

	Context("when the os args include the encryption key", func() {
		JustBeforeEach(func() {
			osArgs = append(osArgs, "--encryption-key", someEncryptionKey)
		})

		It("Uses the encryption key in the os arg instead of the config file", func() {
			rootConfig, err := configPkg.NewConfig(osArgs)
			Expect(err).NotTo(HaveOccurred())

			Expect(rootConfig.SymmetricKey).To(Equal(someEncryptionKey))
		})
	})

	It("Has data for the Instances", func() {
		rootConfig, err := configPkg.NewConfig(osArgs)
		Expect(err).NotTo(HaveOccurred())

		Expect(rootConfig.Instances[0].Address).To(Equal("fakeIp"))
		Expect(rootConfig.Instances[0].UUID).To(Equal("some-uuid"))
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

var _ = Describe("Config.HTTPClient", func() {
	It("creates a http client", func() {
		cfg := configPkg.Config{}
		client := cfg.HTTPClient()
		Expect(client).ToNot(BeNil())
		Expect(client).To(BeAssignableToTypeOf(&http.Client{}))
		Expect(client.Transport).To(BeZero())
	})

	When("backend tls is configured", func() {
		var caPEM string
		BeforeEach(func() {
			ca, err := certtest.BuildCA("serverCA")
			Expect(err).ToNot(HaveOccurred())
			caBytes, err := ca.CertificatePEM()
			Expect(err).ToNot(HaveOccurred())

			caPEM = string(caBytes)
		})

		It("creates an http client w/ a valid TLSClientConfig", func() {
			cfg := configPkg.Config{
				BackendTLS: configPkg.BackendTLS{
					Enabled:    true,
					ServerName: "something",
					CA:         caPEM,
				},
			}
			client := cfg.HTTPClient()
			Expect(client).To(BeAssignableToTypeOf(&http.Client{}))

			Expect(client.Transport).To(BeAssignableToTypeOf(&http.Transport{}))

			tlsConfig := client.Transport.(*http.Transport).TLSClientConfig

			Expect(tlsConfig.ServerName).To(Equal("something"))
			Expect(tlsConfig.RootCAs).ToNot(BeNil())
			Expect(tlsConfig.RootCAs.Subjects()).To(HaveLen(1))
		})
	})
})
