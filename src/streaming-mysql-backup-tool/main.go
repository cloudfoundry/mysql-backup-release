package main

import (
	"net/http"
	"os"
	"strconv"

	"github.com/cloudfoundry/streaming-mysql-backup-tool/api"
	c "github.com/cloudfoundry/streaming-mysql-backup-tool/config"
	"github.com/cloudfoundry/streaming-mysql-backup-tool/middleware"
	"github.com/cloudfoundry/streaming-mysql-backup-tool/xtrabackup"

	"code.cloudfoundry.org/lager/v3"
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

	var backupHandler http.Handler = &api.BackupHandler{
		BackupWriter: xtrabackup.Writer{
			DefaultsFile: config.XtraBackup.DefaultsFile,
			TmpDir:       config.XtraBackup.TmpDir,
			Logger:       config.Logger,
		},
		Logger: logger,
	}

	if !config.TLS.EnableMutualTLS {
		backupHandler = middleware.BasicAuth(backupHandler, config.Credentials.Username, config.Credentials.Password)
	}

	mux.Handle("/backup", backupHandler)

	pidfile, err := os.Create(config.PidFile)
	if err != nil {
		logger.Fatal("Failed to create a file", err)
	}

	_ = os.WriteFile(pidfile.Name(), []byte(strconv.Itoa(os.Getpid())), 0644)

	logger.Info("Starting server with configuration", lager.Data{
		"address": config.BindAddress,
	})

	httpServer := &http.Server{
		Addr:      config.BindAddress,
		Handler:   mux,
		TLSConfig: config.TLS.Config,
	}
	err = httpServer.ListenAndServeTLS("", "")
	logger.Fatal("Streaming backup tool has exited with an error", err)
}
