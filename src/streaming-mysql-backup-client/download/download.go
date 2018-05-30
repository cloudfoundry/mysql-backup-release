package download

import (
	"io"
	"net/http"
	"os"

	"fmt"
	"sync"
	"time"

	"crypto/tls"

	"code.cloudfoundry.org/lager"
	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	"streaming-mysql-backup-client/clock"
	"streaming-mysql-backup-client/config"
)

type DownloadBackup interface {
	DownloadBackup(url string, backupWriter StreamedWriter) error
	TrailerKey() string
}

type HttpDownloadBackup struct {
	logger lager.Logger
	clock  clock.Clock
	config config.Config
}

func NewDownloaderFromCredentials(username, password string, tlsConfig *tls.Config) DownloadBackup {
	logger := lager.NewLogger("streaming-mysql-backup-client")
	logger.RegisterSink(lager.NewWriterSink(os.Stdout, lager.INFO))

	return DefaultDownloadBackup(clock.DefaultClock(), config.Config{
		Credentials: config.Credentials{
			Username: username,
			Password: password,
		},
		Certificates: config.Certificates{
			TlsConfig: tlsConfig,
		},
		Logger: logger,
	})
}

func DefaultDownloadBackup(clock clock.Clock, config config.Config) DownloadBackup {
	return &HttpDownloadBackup{
		logger: config.Logger,
		clock:  clock,
		config: config,
	}
}

type trackingReader struct {
	r         io.Reader
	bytesRead int
	sync.Mutex
}

func (tr *trackingReader) getBytesRead() int {
	tr.Lock()
	defer tr.Unlock()

	return tr.bytesRead
}

func (tr *trackingReader) incrementBytesRead(bytes int) {
	tr.Lock()
	defer tr.Unlock()

	tr.bytesRead += bytes
}

func (tr *trackingReader) Read(p []byte) (n int, err error) {
	n, err = tr.r.Read(p)
	tr.incrementBytesRead(n)

	return
}

func (_ *HttpDownloadBackup) TrailerKey() string {
	return http.CanonicalHeaderKey("X-Backup-Error")
}

type StreamedWriter interface {
	WriteStream(reader io.Reader) error
}

func (this *HttpDownloadBackup) DownloadBackup(backupURL string, backupWriter StreamedWriter) error {
	this.logger.Info("Starting to take backup", lager.Data{
		"url": backupURL,
	})

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: this.config.Certificates.TlsConfig,
		},
	}

	request, err := http.NewRequest("GET", backupURL, nil)
	if err != nil {
		this.logger.Error("Failed to create http request", err)
		return errors.WithStack(err)
	}

	request.SetBasicAuth(this.config.Credentials.Username, this.config.Credentials.Password)
	resp, err := httpClient.Do(request)
	if err != nil {
		this.logger.Error("Failed to make http request", err)
		return errors.WithStack(err)
	}

	/*
	* http.Get() does not throw an error for non-2xx error
	* so using resp block to catch this condition
	 */
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			err = errors.New("Backup endpoint return Unauthorized with provided credentials")
		case http.StatusInternalServerError:
			err = errors.New("Backup endpoint returned Internal Server Error")
		default:
			err = errors.New("Non-200 http Response")
		}

		this.logger.Error("Response returned non-200", err, lager.Data{
			"response status": resp.Status,
		})
		return err
	}
	defer resp.Body.Close()
	trackingReader := &trackingReader{r: resp.Body}

	copyErrChan := make(chan error)
	go func() {
		this.logger.Debug("Copying response body to backup writer")
		err = backupWriter.WriteStream(trackingReader)
		copyErrChan <- err
	}()

	var copyErr error
	done := false
	for done == false {
		select {
		case <-this.clock.After(1 * time.Minute):
			this.logger.Info(fmt.Sprintf("Downloaded %s of backup so far", humanize.Bytes(uint64(trackingReader.getBytesRead()))))
		case copyErr = <-copyErrChan:
			done = true
		}
	}

	if copyErr != nil {
		this.logger.Error("Failed to copy response to writer", copyErr)
		return errors.WithStack(err)
	}

	errorMessage := resp.Trailer.Get(this.TrailerKey())
	if len(errorMessage) > 0 {
		err := errors.New(errorMessage)
		this.logger.Error("The download was incomplete", err)
		return err
	}

	return nil
}
