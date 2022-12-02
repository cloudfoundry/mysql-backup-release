package api

import (
	"encoding/json"
	"io"
	"net/http"

	"code.cloudfoundry.org/lager"
)

const TrailerKey = "X-Backup-Error"

type BackupHandler struct {
	BackupWriter BackupWriter
	Logger       lager.Logger
}

type BackupWriter interface {
	StreamTo(format string, w io.Writer) error
}

func (b *BackupHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var format = "tar"

	switch f := req.URL.Query().Get("format"); f {
	case "":
		format = "tar"
	case "xbstream", "tar":
		format = f
	default:
		b.Logger.Info("invalid request format", lager.Data{"format": f})
		w.WriteHeader(http.StatusBadRequest)
		msg, _ := json.Marshal(map[string]string{"error": "invalid backup format '" + f + "' requested"})
		_, _ = w.Write(msg)
		return
	}

	b.Logger.Info("Responding to request", lager.Data{
		"url":    req.URL.String(),
		"method": req.Method,
	})

	// NOTE: We set this in the Header because of the HTTP spec
	// http://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html#sec14.40
	// Even though we cannot test it, because the `net/http.Get()` strips
	// "Trailer" out of the Header
	w.Header().Set("Trailer", TrailerKey)
	w.Header().Set("Content-Type", "application/octet-stream; format="+format)

	var trailerValue string
	if err := b.BackupWriter.StreamTo(format, w); err != nil {
		b.Logger.Error("streaming backup failed", err)
		trailerValue = err.Error()
	}

	w.Header().Set(TrailerKey, trailerValue)
}
