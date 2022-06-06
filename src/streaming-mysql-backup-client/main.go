package main

import (
	"os"

	"github.com/cloudfoundry/streaming-mysql-backup-client/client"
	"github.com/cloudfoundry/streaming-mysql-backup-client/clock"
	"github.com/cloudfoundry/streaming-mysql-backup-client/config"
	"github.com/cloudfoundry/streaming-mysql-backup-client/download"
	"github.com/cloudfoundry/streaming-mysql-backup-client/galera_agent_caller"
	"github.com/cloudfoundry/streaming-mysql-backup-client/prepare"
	"github.com/cloudfoundry/streaming-mysql-backup-client/tarpit"
)

func main() {

	rootConfig, err := config.NewConfig(os.Args)
	logger := rootConfig.Logger

	if err != nil {
		logger.Fatal("Error parsing config file", err)
	}

	c := client.NewClient(
		*rootConfig,
		tarpit.NewSystemTarClient(),
		prepare.DefaultBackupPreparer(),
		download.DefaultDownloadBackup(clock.DefaultClock(), *rootConfig),
		&galera_agent_caller.GaleraAgentCaller{
			GaleraAgentPort:  (*rootConfig).GaleraAgentPort,
			GaleraBackendTLS: (*rootConfig).BackendTLS,
		},
	)
	if err := c.Execute(); err != nil {
		logger.Fatal("All backups failed. Not able to generate a valid backup artifact. See error(s) below: %s", err)
	}
}
