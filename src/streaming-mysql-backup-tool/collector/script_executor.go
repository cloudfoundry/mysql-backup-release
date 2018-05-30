package collector

import "os/exec"

type executor struct{}

const CollectionScriptBOSHPath = "/var/vcap/jobs/streaming-mysql-backup-tool/bin/simple_collect.sh"

func NewScriptExecutor() *executor {
	return &executor{}
}

func (e *executor) Execute() error {
	cmd := &exec.Cmd{
		Path: CollectionScriptBOSHPath,
	}

	return cmd.Run()
}
