package api

import (
	"errors"
	"net/http"
	"os/exec"

	"code.cloudfoundry.org/lager"
	"streaming-mysql-backup-tool/commandexecutor"
)

var TrailerKey = http.CanonicalHeaderKey("X-Backup-Error")

type BackupHandler struct {
	CommandGenerator   func() *exec.Cmd
	CollectorGenerator func() Collector
	Logger             lager.Logger
}

//go:generate counterfeiter . Collector
type Collector interface {
	Start()
	Stop()
}

func (b *BackupHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	command := b.CommandGenerator()

	b.Logger.Info("Responding to request", lager.Data{
		"url":    request.URL,
		"method": request.Method,
		"body":   request.Body,
	})

	collector := b.CollectorGenerator()
	go collector.Start()
	defer collector.Stop()

	// NOTE: We set this in the Header because of the HTTP spec
	// http://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html#sec14.40
	// Even though we cannot test it, because the `net/http.Get()` strips
	// "Trailer" out of the Header
	writer.Header().Set("Trailer", TrailerKey)

	executor := commandexecutor.NewCommandExecutor(command, writer, b.LoggerWriter(), b.Logger)

	b.Logger.Info("Executing command", lager.Data{
		"command": command.Args[0],
	})
	err := executor.Run()

	errorString := ""
	if err != nil {
		errorString = err.Error()

		b.Logger.Info("Execution of the command Failed", lager.Data{
			"error-string": errorString,
		})
	}

	writer.Header().Set(TrailerKey, errorString)
}

func (b *BackupHandler) LoggerWriter() *LoggerWriter {
	return NewLoggerWriter(b.Logger)
}

//////////////////////////////////////////////////

type LoggerWriter struct {
	logger lager.Logger
}

func NewLoggerWriter(logger lager.Logger) *LoggerWriter {
	return &LoggerWriter{logger: logger}
}

func (lw *LoggerWriter) Write(p []byte) (int, error) {
	err := errors.New(string(p[:]))

	lw.logger.Error("StdErr Pipe", err)

	return len(p), nil
}
