package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"streaming-mysql-backup-tool/api"
	"streaming-mysql-backup-tool/collector"
	"streaming-mysql-backup-tool/config"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/tlsconfig"
	"github.com/cloudfoundry-incubator/switchboard/api/middleware"
)

func main() {
	config, err := config.NewConfig(os.Args)
	logger := config.Logger

	if err != nil {
		logger.Fatal("Failed to read config", err, lager.Data{
			"config": config,
		})
	}

	mux := http.NewServeMux()

	wrappedMux := middleware.Chain{
		middleware.NewBasicAuth(config.Credentials.Username, config.Credentials.Password),
	}.Wrap(mux)

	backupHandler := &api.BackupHandler{
		CommandGenerator:   config.Cmd,
		Logger:             logger,
		CollectorGenerator: func() api.Collector { return collector.NewCollector(collector.NewScriptExecutor(), logger) },
	}

	mux.Handle("/backup", backupHandler)

	pidfile, err := os.Create(config.PidFile)
	if err != nil {
		logger.Fatal("Failed to create a file", err)
	}

	ioutil.WriteFile(pidfile.Name(), []byte(strconv.Itoa(os.Getpid())), 0644)

	logger.Info("Starting server with configuration", lager.Data{
		"port": config.Port,
	})

	tlsConfig, err := tlsconfig.Build(
		tlsconfig.WithIdentityFromFile(config.Certificates.Cert, config.Certificates.Key),
	).Server()
	if err != nil {
		logger.Fatal("Failed to construct mTLS server", err)
	}

	httpServer := &http.Server{
		Addr:      fmt.Sprintf(":%d", config.Port),
		Handler:   wrappedMux,
		TLSConfig: tlsConfig,
	}
	err = httpServer.ListenAndServeTLS(config.Certificates.Cert, config.Certificates.Key)

	logger.Fatal("Streaming backup tool has exited with an error", err)
}
