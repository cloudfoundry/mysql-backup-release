package config

import (
	"flag"

	"crypto/tls"
	"os"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagerflags"
	"code.cloudfoundry.org/tlsconfig"
	service_config "github.com/pivotal-cf-experimental/service-config"
	validator "gopkg.in/validator.v2"
)

type Config struct {
	Ips                    []string     `yaml:"Ips" validate:"nonzero"`
	BackupServerPort       int          `yaml:"BackupServerPort"`
	BackupAllMasters       bool         `yaml:"BackupAllMasters"`
	BackupFromInactiveNode bool         `yaml:"BackupFromInactiveNode"`
	GaleraAgentPort        int          `yaml:"GaleraAgentPort"`
	Credentials            Credentials  `yaml:"Credentials" validate:"nonzero"`
	Certificates           Certificates `yaml:"Certificates" validate:"nonzero"`
	TmpDir                 string       `yaml:"TmpDir" validate:"nonzero"`
	OutputDir              string       `yaml:"OutputDir" validate:"nonzero"`
	SymmetricKey           string       `yaml:"SymmetricKey" validate:"nonzero"`
	Logger                 lager.Logger
	MetadataFields         map[string]string
}

type Credentials struct {
	Username string `yaml:"Username" validate:"nonzero"`
	Password string `yaml:"Password" validate:"nonzero"`
}

type Certificates struct {
	CACert     string `yaml:"CACert" validate:"nonzero"`
	ServerName string `yaml:"ServerName"`
	ClientCert string `yaml:"ClientCert" validate:"nonzero"`
	ClientKey  string `yaml:"ClientKey" validate:"nonzero"`
	TlsConfig  *tls.Config
}

func (c Config) Validate() error {
	return validator.Validate(c)
}

func NewConfig(osArgs []string) (*Config, error) {
	var rootConfig Config

	rootConfig.MetadataFields = make(map[string]string)

	binaryName := osArgs[0]
	configurationOptions := osArgs[1:]

	serviceConfig := service_config.New()
	flags := flag.NewFlagSet(binaryName, flag.ExitOnError)

	lagerflags.AddFlags(flags)
	rootConfig.Logger, _ = lagerflags.New(binaryName)
	rootConfig.Logger.RegisterSink(lager.NewWriterSink(os.Stderr, lager.ERROR))

	serviceConfig.AddFlags(flags)
	flags.Parse(configurationOptions)

	err := serviceConfig.Read(&rootConfig)
	if err != nil {
		return nil, err
	}

	err = rootConfig.CreateTlsConfig()
	if err != nil {
		return nil, err
	}

	return &rootConfig, nil
}

func (this *Config) CreateTlsConfig() error {
	newTLSConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(this.Certificates.ClientCert, this.Certificates.ClientKey),
	).Client(
		tlsconfig.WithAuthorityFromFile(this.Certificates.CACert),
		tlsconfig.WithServerName(this.Certificates.ServerName),
	)
	if err != nil {
		return err
	}

	this.Certificates.TlsConfig = newTLSConfig

	return nil
}
