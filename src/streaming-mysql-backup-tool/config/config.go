package config

import (
	"flag"
	"os/exec"
	"strings"

	"code.cloudfoundry.org/cflager"
	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf-experimental/service-config"
	"gopkg.in/validator.v2"
)

type Config struct {
	Command      string       `yaml:"Command" validate:"nonzero"`
	Port         int          `yaml:"Port" validate:"nonzero"`
	PidFile      string       `yaml:"PidFile" validate:"nonzero"`
	Credentials  Credentials  `yaml:"Credentials" validate:"nonzero"`
	Certificates Certificates `yaml:"Certificates" validate:"nonzero"`
	Logger       lager.Logger
}

type Credentials struct {
	Username string `yaml:"Username" validate:"nonzero"`
	Password string `yaml:"Password" validate:"nonzero"`
}

type Certificates struct {
	Cert string `yaml:"Cert" validate:"nonzero"`
	Key  string `yaml:"Key" validate:"nonzero"`
}

func (c Config) Validate() error {
	return validator.Validate(c)
}

func NewConfig(osArgs []string) (*Config, error) {
	var rootConfig Config

	binaryName := osArgs[0]
	configurationOptions := osArgs[1:]

	serviceConfig := service_config.New()
	flags := flag.NewFlagSet(binaryName, flag.ExitOnError)

	cflager.AddFlags(flags)

	serviceConfig.AddDefaults(Config{
		Port: 8081,
	})

	serviceConfig.AddFlags(flags)
	flags.Parse(configurationOptions)

	err := serviceConfig.Read(&rootConfig)
	rootConfig.Logger, _ = cflager.New(binaryName)

	return &rootConfig, err
}

func (c Config) Cmd() *exec.Cmd {
	fields := strings.Fields(c.Command)
	return exec.Command(fields[0], fields[1:]...)
}
