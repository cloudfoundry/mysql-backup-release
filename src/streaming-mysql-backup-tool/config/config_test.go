package config_test

import (
	"fmt"

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
})
