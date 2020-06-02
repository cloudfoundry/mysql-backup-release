package api_test

import (
	"net/http"
	"net/http/httptest"
	"os/exec"

	"code.cloudfoundry.org/lager/lagertest"
	. "github.com/cloudfoundry/streaming-mysql-backup-tool/api"
	"github.com/cloudfoundry/streaming-mysql-backup-tool/api/apifakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("BackupHandler", func() {
	var (
		command            *exec.Cmd
		testLogger         *lagertest.TestLogger
		fakeCollector      *apifakes.FakeCollector
		backupHandler      *BackupHandler
		fakeResponseWriter *httptest.ResponseRecorder
		request            *http.Request
		err                error
	)

	BeforeEach(func() {
		fakeCollector = new(apifakes.FakeCollector)

		command = exec.Command("some-path")
		testLogger = lagertest.NewTestLogger("collector-test")
		backupHandler = &BackupHandler{
			CommandGenerator:   func() *exec.Cmd { return command },
			CollectorGenerator: func() Collector { return fakeCollector },
			Logger:             testLogger,
		}
		fakeResponseWriter = httptest.NewRecorder()
	})

	It("executes the command while collecting information", func() {
		request, err = http.NewRequest("GET", "/backups", nil)
		Expect(err).NotTo(HaveOccurred())

		backupHandler.ServeHTTP(fakeResponseWriter, request)
		Eventually(fakeCollector.StartCallCount).Should(Equal(1))
		Eventually(fakeCollector.StopCallCount).Should(Equal(1))
	})
})
