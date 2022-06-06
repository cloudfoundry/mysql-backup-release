package config

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"net/http"
	"os"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagerflags"
	"code.cloudfoundry.org/tlsconfig"
	service_config "github.com/pivotal-cf-experimental/service-config"
	"github.com/pkg/errors"
)

type Config struct {
	Instances              []Instance  `yaml:"Instances" validate:"nonzero"`
	BackupServerPort       int         `yaml:"BackupServerPort"`
	BackupAllMasters       bool        `yaml:"BackupAllMasters"`
	BackupFromInactiveNode bool        `yaml:"BackupFromInactiveNode"`
	GaleraAgentPort        int         `yaml:"GaleraAgentPort"`
	Credentials            Credentials `yaml:"Credentials" validate:"nonzero"`
	TmpDir                 string      `yaml:"TmpDir" validate:"nonzero"`
	OutputDir              string      `yaml:"OutputDir" validate:"nonzero"`
	SymmetricKey           string      `yaml:"SymmetricKey" validate:"nonzero"`
	TLS                    TLSConfig   `yaml:"TLS"`
	Logger                 lager.Logger
	MetadataFields         map[string]string
	BackendTLS             BackendTLS `yaml:"BackendTLS"`
}

func (c Config) HTTPClient() *http.Client {
	httpClient := &http.Client{}
	if c.BackendTLS.Enabled {
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM([]byte(c.BackendTLS.CA))
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				ServerName: c.BackendTLS.ServerName,
				RootCAs:    certPool,
			},
		}
	}
	return httpClient
}

type Instance struct {
	Address string `yaml:"Address"`
	UUID    string `yaml:"UUID"`
}

type Credentials struct {
	Username string `yaml:"Username" validate:"nonzero"`
	Password string `yaml:"Password" validate:"nonzero"`
}

type TLSConfig struct {
	EnableMutualTLS bool        `yaml:"EnableMutualTLS"`
	ServerCACert    string      `yaml:"ServerCACert" validate:"nonzero"`
	ServerName      string      `yaml:"ServerName"`
	ClientCert      string      `yaml:"ClientCert" validate:"nonzero"`
	ClientKey       string      `yaml:"ClientKey" validate:"nonzero"`
	Config          *tls.Config `yaml:"-"`
}

type BackendTLS struct {
	Enabled            bool   `yaml:"Enabled"`
	ServerName         string `yaml:"ServerName"`
	CA                 string `yaml:"CA"`
	InsecureSkipVerify bool   `yaml:"InsecureSkipVerify"`
}

func (t *TLSConfig) unmarshalTLSConfig() error {
	var certConfig tlsconfig.Config

	if t.EnableMutualTLS {
		clientCert, err := tls.X509KeyPair([]byte(t.ClientCert), []byte(t.ClientKey))
		if err != nil {
			return errors.Wrap(err, `failed to load client certificate or private key`)
		}

		certConfig = tlsconfig.Build(
			tlsconfig.WithInternalServiceDefaults(),
			tlsconfig.WithIdentity(clientCert),
		)
	} else {
		certConfig = tlsconfig.Build(
			tlsconfig.WithInternalServiceDefaults(),
		)
	}

	serverCA := x509.NewCertPool()

	if ok := serverCA.AppendCertsFromPEM([]byte(t.ServerCACert)); !ok {
		return errors.New(`unable to load server CA certificate`)
	}

	newTLSConfig, err := certConfig.Client(
		tlsconfig.WithAuthority(serverCA),
		tlsconfig.WithServerName(t.ServerName),
	)
	if err != nil {
		return errors.Wrap(err, `failed to build client TLS config`)
	}

	t.Config = newTLSConfig

	return nil
}

func NewConfig(osArgs []string) (*Config, error) {
	var rootConfig Config

	rootConfig.MetadataFields = make(map[string]string)

	binaryName := osArgs[0]
	executableArgs := osArgs[1:]

	serviceConfig := service_config.New()
	flags := flag.NewFlagSet(binaryName, flag.ExitOnError)
	encryptionKey := flags.String("encryption-key", "", "Key used to encrypt the backup")

	lagerflags.AddFlags(flags)
	rootConfig.Logger, _ = lagerflags.New(binaryName)
	rootConfig.Logger.RegisterSink(lager.NewWriterSink(os.Stderr, lager.ERROR))

	serviceConfig.AddFlags(flags)
	_ = flags.Parse(executableArgs)

	err := serviceConfig.Read(&rootConfig)
	if err != nil {
		return &rootConfig, err
	}

	if err := rootConfig.TLS.unmarshalTLSConfig(); err != nil {
		return &rootConfig, err
	}

	if *encryptionKey != "" {
		rootConfig.SymmetricKey = *encryptionKey
	}

	return &rootConfig, nil
}
