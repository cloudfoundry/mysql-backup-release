package config

import (
	"flag"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagerflags"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/pivotal-cf-experimental/service-config"
	"gopkg.in/validator.v2"
	"io/ioutil"
	"os"
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
	certPool := x509.NewCertPool()
	dat, err := ioutil.ReadFile(this.Certificates.CACert)
	if err != nil {
		return err
	}

	if ok := certPool.AppendCertsFromPEM(dat); !ok {
		err := errors.New("unable to parse and append ca certificate")
		return err
	}

	this.Certificates.TlsConfig = &tls.Config{
		RootCAs:    certPool,
		ServerName: this.Certificates.ServerName,
	}

	return nil
}
