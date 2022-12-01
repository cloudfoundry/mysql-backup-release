package api_test

import (
	"net/http"
	"net/http/httptest"
	"os/exec"

	"code.cloudfoundry.org/lager/lagertest"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/cloudfoundry/streaming-mysql-backup-tool/api"
)

var _ = Describe("BackupHandler", func() {
	var (
		command            *exec.Cmd
		testLogger         *lagertest.TestLogger
		backupHandler      *BackupHandler
		fakeResponseWriter *httptest.ResponseRecorder
		request            *http.Request
		err                error
	)

	BeforeEach(func() {
		command = exec.Command("some-path")
		testLogger = lagertest.NewTestLogger("collector-test")
		backupHandler = &BackupHandler{
			CommandGenerator: func() *exec.Cmd { return command },
			Logger:           testLogger,
		}
		fakeResponseWriter = httptest.NewRecorder()
	})

	It("executes the command while collecting information", func() {
		request, err = http.NewRequest("GET", "/backups", nil)
		Expect(err).NotTo(HaveOccurred())

		backupHandler.ServeHTTP(fakeResponseWriter, request)
	})
})
