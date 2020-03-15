package main

import (
	"os"

	c "github.com/cloudfoundry/streaming-mysql-backup-client/client"
	"github.com/cloudfoundry/streaming-mysql-backup-client/config"
)

func main() {

	rootConfig, err := config.NewConfig(os.Args)
	logger := rootConfig.Logger

	if err != nil {
		logger.Fatal("Error parsing config file", err)
	}

	client := c.DefaultClient(*rootConfig)
	if err := client.Execute(); err != nil {
		logger.Fatal("All backups failed. Not able to generate a valid backup artifact. See error(s) below: %s", err)
	}
}
