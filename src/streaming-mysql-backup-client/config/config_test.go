package config_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-cf-experimental/service-config/test_helpers"
	configPkg "streaming-mysql-backup-client/config"
)

var _ = Describe("ClientConfig", func() {
	var (
		rootConfig    *configPkg.Config
		configuration string
	)

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
			configuration = `{
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
					"CACert": "fixtures/CertAuth.crt",
				},
				"TmpDir": "fakeTmp",
				"OutputDir": "fakeOutput",
				"SymmetricKey": "fakeKey",
			}`
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
	})

	Describe("TlsConfig", func() {
		BeforeEach(func() {
			configuration = `{
				"Certificates": {
					"CACert": "fixtures/CertAuth.crt",
					"ServerName": "myServerName",
				}
			}`
		})

		It("includes the CACert as a string", func() {
			Expect(rootConfig.Certificates.TlsConfig.RootCAs.Subjects()[0]).To(ContainSubstring("CertAuth"))
		})

		It("includes the server name", func() {
			Expect(rootConfig.Certificates.TlsConfig.ServerName).To(Equal("myServerName"))
		})
	})

	Describe("CreateTlsConfig", func() {
		BeforeEach(func() {
			configuration = `{
				"Certificates": {
					"CACert": "fixtures/CertAuth.crt",
					"ServerName": "myServerName",
				}
			}`
		})

		Context("When certificates are invalid", func() {
			Context("When certificate file does not exist", func() {
				It("Returns an error", func() {
					rootConfig.Certificates.CACert = "invalid_path"
					err := rootConfig.CreateTlsConfig()
					Expect(err).To(MatchError("open invalid_path: no such file or directory"))
				})
			})

			Context("When CA file is an invalid certificate", func() {
				It("Returns an error", func() {
					rootConfig.Certificates.CACert = "fixtures/InvalidCert.crt"
					err := rootConfig.CreateTlsConfig()
					Expect(err).To(MatchError("unable to parse and append ca certificate"))
				})
			})

		})
	})
})
