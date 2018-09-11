package config_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
				}
			}`
		})

		It("does not return error on valid config", func() {
			err := rootConfig.Validate()
			Expect(err).NotTo(HaveOccurred())
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
