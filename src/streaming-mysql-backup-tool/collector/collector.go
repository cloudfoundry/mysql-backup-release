package collector

import "code.cloudfoundry.org/lager"

//go:generate counterfeiter . ScriptExecutor
type ScriptExecutor interface {
	Execute() error
}

type Collector struct {
	stopChan       chan struct{}
	scriptExecutor ScriptExecutor
	logger         lager.Logger
}

func NewCollector(scriptExecutor ScriptExecutor, logger lager.Logger) *Collector {
	return &Collector{
		scriptExecutor: scriptExecutor,
		stopChan:       make(chan struct{}),
		logger:         logger,
	}
}

func (c *Collector) Start() {
	c.logger.Debug("Starting collection script")
	for {
		select {
		case <-c.stopChan:
			c.logger.Debug("Done running collection script")
			return
		default:
			c.logger.Debug("Starting one run of script")
			err := c.scriptExecutor.Execute()
			if err != nil {
				c.logger.Error("Error executing collection script", err)
			}
			c.logger.Debug("Finished one run of script")
		}
	}
}

func (c *Collector) Stop() {
	c.logger.Debug("Stopping collection script")
	close(c.stopChan)
}
