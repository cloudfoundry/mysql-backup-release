package main

import (
	"io/ioutil"
	"net/http"
	"os"
	"strconv"

	"github.com/cloudfoundry-incubator/switchboard/api/middleware"

	"github.com/cloudfoundry/streaming-mysql-backup-tool/api"
	c "github.com/cloudfoundry/streaming-mysql-backup-tool/config"
	"github.com/cloudfoundry/streaming-mysql-backup-tool/xtrabackup"

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
		BackupWriter: xtrabackup.Writer{
			DefaultsFile: config.XtraBackup.DefaultsFile,
			TmpDir:       config.XtraBackup.TmpDir,
			Logger:       config.Logger,
		},
		Logger: logger,
	}

	mux.Handle("/backup", backupHandler)

	pidfile, err := os.Create(config.PidFile)
	if err != nil {
		logger.Fatal("Failed to create a file", err)
	}

	_ = ioutil.WriteFile(pidfile.Name(), []byte(strconv.Itoa(os.Getpid())), 0644)

	logger.Info("Starting server with configuration", lager.Data{
		"address": config.BindAddress,
	})

	httpServer := &http.Server{
		Addr:      config.BindAddress,
		Handler:   wrappedMux,
		TLSConfig: config.TLS.Config,
	}
	err = httpServer.ListenAndServeTLS("", "")
	logger.Fatal("Streaming backup tool has exited with an error", err)
}
