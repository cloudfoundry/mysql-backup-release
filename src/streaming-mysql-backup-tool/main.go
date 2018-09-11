package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"

	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry-incubator/switchboard/api/middleware"
	"streaming-mysql-backup-tool/api"
	"streaming-mysql-backup-tool/collector"
	"streaming-mysql-backup-tool/config"
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

	err = http.ListenAndServe(fmt.Sprintf(":%d", config.Port), wrappedMux)
	logger.Fatal("Streaming backup tool has exited with an error", err)
}
