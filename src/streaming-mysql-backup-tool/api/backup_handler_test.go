package api_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"

	"code.cloudfoundry.org/lager/lagertest"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/cloudfoundry/streaming-mysql-backup-tool/api"
)

var _ = Describe("BackupHandler", func() {
	var (
		testLogger         *lagertest.TestLogger
		backupHandler      *BackupHandler
		fakeBackupWriter   *stubBackupWriter
		fakeResponseWriter *httptest.ResponseRecorder
		request            *http.Request
		err                error
	)

	BeforeEach(func() {
		fakeBackupWriter = &stubBackupWriter{}
		testLogger = lagertest.NewTestLogger("handler-test")
		backupHandler = &BackupHandler{
			BackupWriter: fakeBackupWriter,
			Logger:       testLogger,
		}
		fakeResponseWriter = httptest.NewRecorder()
	})

	It("streams a backup via http", func() {
		request, err = http.NewRequest("GET", "/backups", nil)
		Expect(err).NotTo(HaveOccurred())

		fakeBackupWriter.content = "some-backup-content"

		backupHandler.ServeHTTP(fakeResponseWriter, request)
		result := fakeResponseWriter.Result()

		body, _ := io.ReadAll(result.Body)
		Expect(string(body)).To(Equal("some-backup-content"))

		Expect(fakeBackupWriter.formatArg).
			To(Equal("xbstream"),
				`Expected the backup format to be "xbstream" but it was not.`)
	})

	It("always sends an http 200 and stores errors in a Trailer Header", func() {
		request, err = http.NewRequest("GET", "/backups", nil)
		Expect(err).NotTo(HaveOccurred())

		fakeBackupWriter.content = "some-backup-content-before-an-error"
		fakeBackupWriter.err = errors.New("some-backup-error")

		backupHandler.ServeHTTP(fakeResponseWriter, request)
		result := fakeResponseWriter.Result()

		Expect(result.StatusCode).To(Equal(http.StatusOK))

		Expect(result.Header.Get("Content-Type")).To(Equal("application/octet-stream; format=xbstream"))

		body, _ := io.ReadAll(result.Body)
		Expect(string(body)).To(Equal("some-backup-content-before-an-error"))

		Expect(result.Trailer.Get(TrailerKey)).To(Equal(`some-backup-error`))
	})

	When("the backup was successful", func() {
		It("still emits a trailer key with an empty value indicating no error", func() {
			request, err = http.NewRequest("GET", "/backups", nil)
			Expect(err).NotTo(HaveOccurred())

			fakeBackupWriter.content = "some-backup-content"

			backupHandler.ServeHTTP(fakeResponseWriter, request)
			result := fakeResponseWriter.Result()

			_, _ = io.ReadAll(result.Body)

			Expect(result.Trailer).To(HaveKeyWithValue(TrailerKey, ConsistOf("")))
		})
	})

	It("defines a canonical TrailerKey constant", func() {
		Expect(TrailerKey).To(Equal(http.CanonicalHeaderKey(TrailerKey)))
	})
})

type stubBackupWriter struct {
	callCount int
	formatArg string
	content   string
	err       error
}

func (f *stubBackupWriter) StreamTo(format string, w io.Writer) error {
	f.callCount++
	f.formatArg = format
	_, _ = w.Write([]byte(f.content))
	return f.err
}

var _ BackupWriter = &stubBackupWriter{}
