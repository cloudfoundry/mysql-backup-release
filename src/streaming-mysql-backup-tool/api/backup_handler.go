package api

import (
	"io"
	"net/http"

	"code.cloudfoundry.org/lager"
)

var TrailerKey = http.CanonicalHeaderKey("X-Backup-Error")

type BackupHandler struct {
	BackupWriter BackupWriter
	Logger       lager.Logger
}

type BackupWriter interface {
	StreamTo(format string, w io.Writer) error
}

func (b *BackupHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	b.Logger.Info("Responding to request", lager.Data{
		"url":    req.URL,
		"method": req.Method,
		"body":   req.Body,
	})

	// NOTE: We set this in the Header because of the HTTP spec
	// http://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html#sec14.40
	// Even though we cannot test it, because the `net/http.Get()` strips
	// "Trailer" out of the Header
	w.Header().Set("Trailer", TrailerKey)
	w.Header().Set("Content-Type", "application/octet-stream; format=xbstream")

	err := b.BackupWriter.StreamTo("xbstream", w)
	errorString := ""
	if err != nil {
		errorString = err.Error()

		b.Logger.Info("Execution of the command Failed", lager.Data{
			"error-string": errorString,
		})
	}

	w.Header().Set(TrailerKey, errorString)
}
