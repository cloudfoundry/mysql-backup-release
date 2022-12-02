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
		testLogger = lagertest.NewTestLogger("collector-test")
		fakeBackupWriter = &stubBackupWriter{}
		backupHandler = &BackupHandler{
			BackupWriter: fakeBackupWriter,
			Logger:       testLogger,
		}
		fakeResponseWriter = httptest.NewRecorder()
	})

	It("defines a canonical TrailerKey constant", func() {
		Expect(TrailerKey).To(Equal(http.CanonicalHeaderKey(TrailerKey)))
	})

	It("uses a trailer key to surface backup errors", func() {
		request, err = http.NewRequest("GET", "/backups", nil)
		Expect(err).NotTo(HaveOccurred())

		fakeBackupWriter.content = "some-data"
		fakeBackupWriter.err = errors.New("some-error")

		backupHandler.ServeHTTP(fakeResponseWriter, request)

		res := fakeResponseWriter.Result()
		Expect(res.Header).NotTo(HaveKey(TrailerKey))
		Expect(res.Trailer).To(HaveKeyWithValue(TrailerKey, ContainElement("some-error")))
	})

	When("the `format` parameter is NOT specified", func() {
		It("sets the Content-Type header to tar by default", func() {
			request, err = http.NewRequest("GET", "/backups", nil)
			Expect(err).NotTo(HaveOccurred())
			backupHandler.ServeHTTP(fakeResponseWriter, request)
			Expect(fakeResponseWriter.Result().Header.Get("Content-Type")).To(Equal(`application/octet-stream; format=tar`))
		})

		It("indicates success by setting an empty Trailer", func() {
			request, err = http.NewRequest("GET", "/backups", nil)
			Expect(err).NotTo(HaveOccurred())
			backupHandler.ServeHTTP(fakeResponseWriter, request)
			response := fakeResponseWriter.Result()
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			Expect(response.Header.Get(TrailerKey)).To(BeEmpty())
		})

		It("delegates to the BackupWriter", func() {
			request, err = http.NewRequest("GET", "/backups", nil)
			Expect(err).NotTo(HaveOccurred())
			backupHandler.ServeHTTP(fakeResponseWriter, request)
			Expect(fakeBackupWriter.callCount).To(Equal(1))
			Expect(fakeBackupWriter.formatArg).To(Equal("tar"))
		})
	})

	When("the `format` parameter is explicitly set to tar", func() {
		It("sets the Content-Type Header to indicate the format", func() {
			request, err = http.NewRequest("GET", "/backups?format=tar", nil)
			Expect(err).NotTo(HaveOccurred())
			backupHandler.ServeHTTP(fakeResponseWriter, request)

			Expect(fakeResponseWriter.Result().Header.Get("Content-Type")).To(Equal(`application/octet-stream; format=tar`))
		})

		It("delegates to the BackupWriter", func() {
			request, err = http.NewRequest("GET", "/backups?format=tar", nil)
			Expect(err).NotTo(HaveOccurred())
			backupHandler.ServeHTTP(fakeResponseWriter, request)
			Expect(fakeBackupWriter.callCount).To(Equal(1))
			Expect(fakeBackupWriter.formatArg).To(Equal("tar"))
		})
	})

	When("the `format` parameter is set to xbstream", func() {
		It("sets the Content-Type Header to indicate the format", func() {
			request, err = http.NewRequest("GET", "/backups?format=xbstream", nil)
			Expect(err).NotTo(HaveOccurred())
			backupHandler.ServeHTTP(fakeResponseWriter, request)

			Expect(fakeResponseWriter.Result().Header.Get("Content-Type")).To(Equal(`application/octet-stream; format=xbstream`))
		})

		It("delegates to the BackupWriter", func() {
			request, err = http.NewRequest("GET", "/backups?format=xbstream", nil)
			Expect(err).NotTo(HaveOccurred())
			backupHandler.ServeHTTP(fakeResponseWriter, request)
			Expect(fakeBackupWriter.callCount).To(Equal(1))
			Expect(fakeBackupWriter.formatArg).To(Equal("xbstream"))
		})
	})

	When("the `format` parameter is set to an invalid value", func() {
		It("return a response indicating a bad requesst", func() {
			request, err = http.NewRequest("GET", "/backups?format=foobar", nil)
			Expect(err).NotTo(HaveOccurred())
			backupHandler.ServeHTTP(fakeResponseWriter, request)

			Expect(fakeResponseWriter.Result().StatusCode).To(Equal(http.StatusBadRequest))

			response, _ := io.ReadAll(fakeResponseWriter.Result().Body)
			Expect(string(response)).To(MatchJSON(`{"error": "invalid backup format 'foobar' requested"}`))
		})
	})

	When("the backup fails halfway through", func() {
		It("has HTTP 200 status code but writes the error to the trailer", func() {
			request, err = http.NewRequest("GET", "/backups", nil)
			Expect(err).NotTo(HaveOccurred())

			fakeBackupWriter.err = errors.New("failed backup error")
			fakeBackupWriter.content = "initial-content"

			backupHandler.ServeHTTP(fakeResponseWriter, request)

			result := fakeResponseWriter.Result()
			Expect(result.StatusCode).To(Equal(http.StatusOK))
			body, err := io.ReadAll(result.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal("initial-content"))
			Expect(result.Trailer.Get(TrailerKey)).To(Equal("failed backup error"))
		})
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
