package config_test

import (
	"code.cloudfoundry.org/tlsconfig/certtest"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	configPkg "streaming-mysql-backup-client/config"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-cf-experimental/service-config/test_helpers"
)

var _ = Describe("ClientConfig", func() {
	var (
		rootConfig    *configPkg.Config
		configuration string

		caPath         string
		certPath       string
		privateKeyPath string
		tmpDir         string
	)

	AfterEach(func() {
		if tmpDir != "" {
			os.RemoveAll(tmpDir)
		}
	})

	BeforeEach(func() {
		ca, err := certtest.BuildCA("serverCA")
		Expect(err).ToNot(HaveOccurred())
		caBytes, err := ca.CertificatePEM()
		Expect(err).ToNot(HaveOccurred())

		certificate, err := ca.BuildSignedCertificate("serverCert")
		Expect(err).ToNot(HaveOccurred())
		certPEM, privateKey, err := certificate.CertificatePEMAndPrivateKey()
		Expect(err).ToNot(HaveOccurred())

		tmpDir, err = ioutil.TempDir("", "backup-tool-tests")
		Expect(err).ToNot(HaveOccurred())

		caPath = filepath.Join(tmpDir, "ca.crt")
		Expect(ioutil.WriteFile(caPath, caBytes, 0666)).To(Succeed())
		certPath = filepath.Join(tmpDir, "cert.crt")
		Expect(ioutil.WriteFile(certPath, certPEM, 0666)).To(Succeed())
		privateKeyPath = filepath.Join(tmpDir, "key.key")
		Expect(ioutil.WriteFile(privateKeyPath, privateKey, 0666)).To(Succeed())
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

	Describe("Validate", func() {
		BeforeEach(func() {
			configuration = fmt.Sprintf(`{
				"Ips": ["fakeIp"],
				"BackupServerPort": 8081,
				"BackupAllMasters": false,
				"BackupFromInactiveNode": false,
				"GaleraAgentPort": null,
				"Credentials":{
					"Username": "fake_username",
					"Password": "fake_password",
				},
				"Certificates": {
					"ClientCert": %q,
					"ClientKey": %q,
					"CACert": %q,
				},
				"TmpDir": "fakeTmp",
				"OutputDir": "fakeOutput",
				"SymmetricKey": "fakeKey",
			}`, certPath, privateKeyPath, caPath)
		})

		It("does not return error on valid config", func() {
			err := rootConfig.Validate()
			Expect(err).NotTo(HaveOccurred())
		})
		It("returns an error if ips is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Ips")
			Expect(err).ToNot(HaveOccurred())
		})
		It("returns an error if username is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Credentials.Username")
			Expect(err).ToNot(HaveOccurred())
		})
		It("returns an error if password is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Credentials.Password")
			Expect(err).ToNot(HaveOccurred())
		})
		It("returns an error if TmpDir is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "TmpDir")
			Expect(err).ToNot(HaveOccurred())
		})
		It("returns an error if OutputDir is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "OutputDir")
			Expect(err).ToNot(HaveOccurred())
		})
		It("returns an error if SymmetricKey is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "SymmetricKey")
			Expect(err).ToNot(HaveOccurred())
		})
		It("returns an error if Certificates is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Certificates")
			Expect(err).ToNot(HaveOccurred())
		})
		It("returns an error if Certificates.Cert is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Certificates.ClientCert")
			Expect(err).ToNot(HaveOccurred())
		})
		It("returns an error if Certificates.Key is blank", func() {
			err := test_helpers.IsRequiredField(rootConfig, "Certificates.ClientKey")
			Expect(err).ToNot(HaveOccurred())
		})

	})

	Describe("CreateTlsConfig", func() {
		BeforeEach(func() {
			configuration = fmt.Sprintf(`{
				"Certificates": {
					"ClientCert": %q,
					"ClientKey": %q,
					"CACert": %q,
					"ServerName": "myServerName"
				}
			}`, certPath, privateKeyPath, caPath)
		})

		Context("When certificates are valid", func() {
			It("Creates a TlsConfig", func() {
				Expect(rootConfig.CreateTlsConfig()).To(Succeed())
				Expect(rootConfig.Certificates.TlsConfig.RootCAs).NotTo(BeNil())
				Expect(rootConfig.Certificates.TlsConfig.ServerName).To(Equal("myServerName"))
				Expect(rootConfig.Certificates.TlsConfig.Certificates).To(HaveLen(1))
			})
		})

		Context("When certificates are invalid", func() {
			Context("When certificate file does not exist", func() {
				It("Returns an error", func() {
					rootConfig.Certificates.CACert = "invalid_path"
					err := rootConfig.CreateTlsConfig()
					Expect(err).To(MatchError("failed to read file invalid_path: open invalid_path: no such file or directory"))
				})
			})

			Context("When CA file is an invalid certificate", func() {
				It("Returns an error", func() {
					rootConfig.Certificates.CACert = privateKeyPath
					err := rootConfig.CreateTlsConfig()
					Expect(err).To(MatchError("unable to load CA certificate at " + privateKeyPath))
				})
			})
		})
	})
})
