package config

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"time"

	"code.cloudfoundry.org/cflager"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/tlsconfig"
	"github.com/pivotal-cf-experimental/service-config"
	"github.com/pkg/errors"
)

type Config struct {
	Port        int         `yaml:"Port" validate:"nonzero"`
	PidFile     string      `yaml:"PidFile" validate:"nonzero"`
	Credentials Credentials `yaml:"Credentials" validate:"nonzero"`
	TLS         TLSConfig   `yaml:"TLS"`
	Logger      lager.Logger
	XtraBackup  XtraBackup `yaml:"XtraBackup"`
}

type XtraBackup struct {
	DefaultsFile string `yaml:"DefaultsFile"`
	TmpDir       string `yaml:"TmpDir"`
}

type Credentials struct {
	Username string `yaml:"Username" validate:"nonzero"`
	Password string `yaml:"Password" validate:"nonzero"`
}

type TLSConfig struct {
	EnableMutualTLS          bool        `yaml:"EnableMutualTLS"`
	RequiredClientIdentities []string    `yaml:"RequiredClientIdentities"`
	ServerCert               string      `yaml:"ServerCert" validate:"nonzero"`
	ServerKey                string      `yaml:"ServerKey" validate:"nonzero"`
	ClientCA                 string      `yaml:"ClientCA" validate:"nonzero"`
	Config                   *tls.Config `yaml:"-"`
}

type ClientCertificateVerifierFunc func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error

func (t *TLSConfig) unmarshalTLSConfig() error {
	serverCert, err := tls.X509KeyPair([]byte(t.ServerCert), []byte(t.ServerKey))
	if err != nil {
		return errors.Wrapf(err, `failed to load server certificate or private key`)
	}

	certConfig := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentity(serverCert),
	)

	var configOptions []tlsconfig.ServerOption

	var verifyPeerCertificate ClientCertificateVerifierFunc

	if t.EnableMutualTLS {
		clientCAPool := x509.NewCertPool()

		if ok := clientCAPool.AppendCertsFromPEM([]byte(t.ClientCA)); !ok {
			return errors.New(`unable to load client CA certificate`)
		}

		configOptions = append(configOptions,
			tlsconfig.WithClientAuthentication(clientCAPool),
		)

		verifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
			for _, name := range t.RequiredClientIdentities {
				opts := x509.VerifyOptions{
					Roots:         clientCAPool,
					CurrentTime:   time.Now(),
					Intermediates: x509.NewCertPool(),
					KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
					DNSName:       name,
				}
				if _, err := verifiedChains[0][0].Verify(opts); err == nil {
					return nil
				}
			}

			return errors.New("invalid client identity in presented client certificate")
		}

	}

	serverTLSConfig, err := certConfig.Server(configOptions...)
	if err != nil {
		return errors.Wrap(err, `failed to build server TLS certConfig`)
	}

	serverTLSConfig.VerifyPeerCertificate = verifyPeerCertificate

	t.Config = serverTLSConfig

	return nil
}

func NewConfig(osArgs []string) (*Config, error) {
	var (
		rootConfig Config
	)

	binaryName := osArgs[0]
	configurationOptions := osArgs[1:]

	serviceConfig := service_config.New()
	flags := flag.NewFlagSet(binaryName, flag.ExitOnError)

	cflager.AddFlags(flags)

	serviceConfig.AddDefaults(Config{
		Port: 8081,
	})

	serviceConfig.AddFlags(flags)
	_ = flags.Parse(configurationOptions)

	err := serviceConfig.Read(&rootConfig)
	rootConfig.Logger, _ = cflager.New(binaryName)
	if err != nil {
		return &rootConfig, err
	}

	if err := rootConfig.TLS.unmarshalTLSConfig(); err != nil {
		return &rootConfig, err
	}

	return &rootConfig, nil
}
