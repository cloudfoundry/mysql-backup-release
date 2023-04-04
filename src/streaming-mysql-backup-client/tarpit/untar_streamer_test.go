package tarpit_test

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/cloudfoundry/streaming-mysql-backup-client/tarpit"

	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	tarFileFixturePath = "fixtures/backup.tar"
)

var _ = Describe("UntarStreamer.WriteStream()", func() {
	var (
		tempDir string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = ioutil.TempDir("", "untarstream")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	It("untars the bytes it receives into the destination directory", func() {
		untarStreamer := tarpit.NewUntarStreamer(tempDir)

		tarFile, err := os.Open(tarFileFixturePath)
		Expect(err).ToNot(HaveOccurred())

		err = untarStreamer.WriteStream(tarFile)
		Expect(err).ToNot(HaveOccurred())

		Expect(fmt.Sprintf("%s/tarfile/hello.txt", tempDir)).To(BeAnExistingFile())
	})

	Context("when the tar command fails", func() {
		It("returns an error with the combined output of the tar command", func() {
			untarStreamer := tarpit.NewUntarStreamer("/some/fake/directory")

			err := untarStreamer.WriteStream(&bytes.Buffer{})
			Expect(err).To(MatchError(ContainSubstring("exit status 2")))
			Expect(err).To(MatchError(ContainSubstring("This does not look like a tar archive")))
		})
	})
})
