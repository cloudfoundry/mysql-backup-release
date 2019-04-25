package download_test

import (
	"code.cloudfoundry.org/tlsconfig/certtest"
	"crypto/subtle"
	"crypto/tls"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/pkg/errors"

	"streaming-mysql-backup-client/clock/fakes"
	"streaming-mysql-backup-client/config"
	"streaming-mysql-backup-client/download"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"

	"code.cloudfoundry.org/lager/lagertest"
)

type bufferWriter struct {
	Buffer *Buffer
	err    error
}

func (bw bufferWriter) WriteStream(reader io.Reader) error {
	if bw.err != nil {
		return bw.err
	}

	_, err := io.Copy(bw.Buffer, reader)
	return err
}

const clockInterval = 50 * time.Millisecond

var _ = Describe("Downloading the backups", func() {

	var (
		downloader           download.DownloadBackup
		logger               lagertest.TestLogger
		fakeClock            *fakes.FakeClock
		test_server          *httptest.Server
		handlerFunc          func(http.ResponseWriter, *http.Request)
		expectedResponseBody = make([]byte, 1024)
		expectedUsername     string
		expectedPassword     string
		trailerError         string
		rootConfig           *config.Config
		bufWriter            bufferWriter
		certificate          tls.Certificate
		backupCA             *certtest.Authority
		otherCA              *certtest.Authority
		tmpDir               string
	)

	AfterEach(func() {
		if tmpDir != "" {
			os.RemoveAll(tmpDir)
		}
	})

	BeforeEach(func() {
		logger = *lagertest.NewTestLogger("backup-client-test")
		fakeClock = &fakes.FakeClock{}
		fakeClock.AfterStub = func(_ time.Duration) <-chan time.Time {
			return time.After(clockInterval)
		}

		bufWriter = bufferWriter{
			Buffer: NewBuffer(),
		}

		var err error
		backupCA, err = certtest.BuildCA("backupCA")
		Expect(err).ToNot(HaveOccurred())
		backupCABytes, err := backupCA.CertificatePEM()
		Expect(err).ToNot(HaveOccurred())

		backupCert, err := backupCA.BuildSignedCertificate("backupCert",
			certtest.WithDomains("expected-server-name"))
		Expect(err).ToNot(HaveOccurred())

		backupCertPEM, privateBackupKey, err := backupCert.CertificatePEMAndPrivateKey()
		Expect(err).ToNot(HaveOccurred())

		tmpDir, err = ioutil.TempDir("", "backup-download-tests")
		Expect(err).ToNot(HaveOccurred())

		backupCAPath := filepath.Join(tmpDir, "backupCA.crt")
		Expect(ioutil.WriteFile(backupCAPath, backupCABytes, 0666)).To(Succeed())
		backupCertPath := filepath.Join(tmpDir, "backup.crt")
		Expect(ioutil.WriteFile(backupCertPath, backupCertPEM, 0666)).To(Succeed())
		backupKeyPath := filepath.Join(tmpDir, "backup.key")
		Expect(ioutil.WriteFile(backupKeyPath, privateBackupKey, 0666)).To(Succeed())

		expectedUsername = "username"
		expectedPassword = "password"
		trailerError = ""

		rootConfig = &config.Config{
			Logger: logger,
			Credentials: config.Credentials{
				Username: expectedUsername,
				Password: expectedPassword,
			},
			Certificates: config.Certificates{
				CACert:     backupCAPath,
				ClientCert: backupCertPath,
				ClientKey:  backupKeyPath,
				ServerName: "expected-server-name",
			},
		}

		err = rootConfig.CreateTlsConfig()
		Expect(err).ToNot(HaveOccurred())

		//this 'happy-path' server handler can be overridden in later BeforeEach blocks
		handlerFunc = func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()

			if ok &&
				secureCompare(username, expectedUsername) &&
				secureCompare(password, expectedPassword) {
			} else {
				w.Header().Set("WWW-Authenticate", "Basic realm=\"Authorization Required\"")
				http.Error(w, "Not Authorized", http.StatusUnauthorized)
				return
			}

			w.Header().Add("Trailer", downloader.TrailerKey())
			writeBody(w, expectedResponseBody)
			writeTrailer(w, downloader.TrailerKey(), trailerError)
		}

		certificate, err = backupCert.TLSCertificate()
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		handler := http.HandlerFunc(handlerFunc)
		test_server = httptest.NewUnstartedServer(handler)

		certPool, err := backupCA.CertPool()
		Expect(err).ToNot(HaveOccurred())

		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{certificate},
			RootCAs:      certPool,
		}

		test_server.TLS = tlsConfig

		test_server.StartTLS()

		downloader = download.DefaultDownloadBackup(fakeClock, *rootConfig)
	})

	AfterEach(func() {
		test_server.Close()
		os.Remove("file.tar")
	})

	Context("when credentials are invalid", func() {
		BeforeEach(func() {
			rootConfig.Credentials.Username = "bad_username"
			rootConfig.Credentials.Password = "bad_password"
		})

		It("Returns a not authorized error", func() {
			err := downloader.DownloadBackup(test_server.URL, bufWriter)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("Unauthorized"))
			Expect(logger.Buffer()).Should(Say(`Unauthorized`))
		})
	})

	Context("when the certificate is signed by a trusted CA", func() {
		Context("and the CN is the expected server name", func() {
			It("downloads a backup and logs", func() {
				expectedResponseBody = []byte("some response body")

				err := downloader.DownloadBackup(test_server.URL, bufWriter)
				Expect(err).ToNot(HaveOccurred())

				Expect(string(bufWriter.Buffer.Contents())).To(Equal("some response body"))
				Expect(logger.Buffer()).Should(Say(`Downloaded`))
			})
		})

		Context("and the CN is not the expected server name", func() {
			BeforeEach(func() {
				var err error
				otherCA, err = certtest.BuildCA("other")
				Expect(err).ToNot(HaveOccurred())

				otherCert, err := otherCA.BuildSignedCertificate("otherCert",
					certtest.WithDomains("other"))
				Expect(err).ToNot(HaveOccurred())
				certificate, err = otherCert.TLSCertificate()
				Expect(err).NotTo(HaveOccurred())

			})

			It("returns an error with a stack", func() {
				err := downloader.DownloadBackup(test_server.URL, bufWriter)
				Expect(reflect.TypeOf(err).String()).To(Equal("*errors.withStack"))
				Expect(err).To(MatchError(ContainSubstring("certificate is valid for other, not expected-server-name")))
			})
		})
	})

	Context("when the certificate is signed by an unknown CA", func() {
		BeforeEach(func() {
			var err error
			otherCA, err = certtest.BuildCA("other")
			Expect(err).ToNot(HaveOccurred())
			otherCABytes, err := otherCA.CertificatePEM()
			Expect(err).ToNot(HaveOccurred())
			otherCAPath := filepath.Join(tmpDir, "otherCA.crt")
			Expect(ioutil.WriteFile(otherCAPath, otherCABytes, 0666)).To(Succeed())

			rootConfig.Certificates.CACert = otherCAPath
			rootConfig.CreateTlsConfig()
		})

		It("returns an error with a stack", func() {
			err := downloader.DownloadBackup(test_server.URL, bufWriter)
			Expect(reflect.TypeOf(err).String()).To(Equal("*errors.withStack"))
			Expect(err).To(MatchError(ContainSubstring("x509: certificate signed by unknown authority")))
		})
	})

	Context("When endpoint doesn't exist", func() {
		BeforeEach(func() {
			handlerFunc = func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "something went wrong", 404)
			}
		})

		It("Returns non-200 error", func() {
			err := downloader.DownloadBackup(test_server.URL, bufWriter)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("Non-200 http Response"))
			Expect(logger.Buffer()).Should(Say(`Response returned non-200`))
		})
	})

	Context("When the backup is incomplete", func() {
		BeforeEach(func() {
			trailerError = "backup was incomplete"
		})

		It("because the download was incomplete", func() {
			err := downloader.DownloadBackup(test_server.URL, bufWriter)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(trailerError))
		})
	})

	Context("When the download was incomplete", func() {
		BeforeEach(func() {
			trailerError = "backup was incomplete"

			handlerFunc = func(w http.ResponseWriter, r *http.Request) {
				username, password, ok := r.BasicAuth()

				if ok &&
					secureCompare(username, expectedUsername) &&
					secureCompare(password, expectedPassword) {
				} else {
					w.Header().Set("WWW-Authenticate", "Basic realm=\"Authorization Required\"")
					http.Error(w, "Not Authorized", http.StatusUnauthorized)
					return
				}

				w.Header().Add("Trailer", downloader.TrailerKey())
				writeBody(w, expectedResponseBody)
				writeTrailer(w, downloader.TrailerKey(), trailerError)
			}
		})

		It("returns the right error with a stack", func() {
			err := downloader.DownloadBackup(test_server.URL, bufWriter)
			Expect(reflect.TypeOf(err).String()).To(Equal("*errors.fundamental"))
			Expect(err).To(MatchError(ContainSubstring(trailerError)))
		})
	})

	Context("When backupWriter.WriteStream fails", func() {
		JustBeforeEach(func() {
			bufWriter.err = errors.New("i am a bad writer")
		})

		It("logs and returns an error with a stack", func() {
			err := downloader.DownloadBackup(test_server.URL, bufWriter)
			Expect(reflect.TypeOf(err).String()).To(Equal("*errors.withStack"))
			Expect(err).To(MatchError("i am a bad writer"))
			Expect(logger.Buffer()).Should(Say("Failed to copy response to writer"))
			Expect(logger.Buffer()).Should(Say("i am a bad writer"))
		})
	})
})

func writeBody(w http.ResponseWriter, bodyContents []byte) {
	w.Write(bodyContents)
	w.(http.Flusher).Flush()
	time.Sleep(clockInterval * 2)
}

func writeTrailer(writer http.ResponseWriter, key string, value string) {
	trailers := http.Header{}
	trailers.Set(key, value)

	// TODO: #99253118 remove this workaround once we move to Go 1.5
	writer.(http.Flusher).Flush()
	conn, buf, _ := writer.(http.Hijacker).Hijack()

	buf.WriteString("0\r\n") // eof
	trailers.Write(buf)

	buf.WriteString("\r\n") // end of trailers
	buf.Flush()
	conn.Close()
}

func secureCompare(a, b string) bool {
	x := []byte(a)
	y := []byte(b)
	return subtle.ConstantTimeCompare(x, y) == 1
}
