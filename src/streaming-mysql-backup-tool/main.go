package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/cloudfoundry-incubator/switchboard/api/middleware"

	"github.com/cloudfoundry/streaming-mysql-backup-tool/api"
	"github.com/cloudfoundry/streaming-mysql-backup-tool/collector"
	c "github.com/cloudfoundry/streaming-mysql-backup-tool/config"

	"code.cloudfoundry.org/lager"
)

func main() {
	config, err := c.NewConfig(os.Args)
	logger := config.Logger

	if err != nil {
		logger.Fatal("Failed to read config", err, lager.Data{
			"config": config,
		})
	}

	mux := http.NewServeMux()

	var wrappedMux http.Handler
	if config.TLS.EnableMutualTLS {
		wrappedMux = mux
	} else {
		wrappedMux = middleware.Chain{
			middleware.NewBasicAuth(config.Credentials.Username, config.Credentials.Password),
		}.Wrap(mux)
	}

	backupHandler := &api.BackupHandler{
		CommandGenerator:   config.Cmd,
		Logger:             logger,
		CollectorGenerator: func() api.Collector { return collector.NewCollector(collector.NewScriptExecutor(), logger) },
	}

	mux.Handle("/backup", backupHandler)

	logger.Info("Starting server with configuration", lager.Data{
		"port": config.Port,
	})

	httpServer := &http.Server{
		Addr:      fmt.Sprintf(":%d", config.Port),
		Handler:   wrappedMux,
		TLSConfig: config.TLS.Config,
	}
	err = httpServer.ListenAndServeTLS("", "")
	logger.Fatal("Streaming backup tool has exited with an error", err)
}
