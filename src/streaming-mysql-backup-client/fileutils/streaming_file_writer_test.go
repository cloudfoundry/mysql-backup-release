package fileutils_test

import (
	"github.com/cloudfoundry/streaming-mysql-backup-client/fileutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"errors"
	"io/ioutil"
	"os"
	"strings"
)

type errorReader struct {
}

func (er errorReader) Read(b []byte) (n int, err error) {
	return 0, errors.New("Reader Error")
}

var _ = Describe("Streaming File Writer", func() {
	var (
		fileWriter fileutils.StreamingFileWriter
		tmpFile    *os.File
		reader     *strings.Reader
	)

	BeforeEach(func() {
		reader = strings.NewReader("some-text")
	})

	Describe("WriteStream()", func() {
		Context("When the file cannot be created", func() {
			BeforeEach(func() {
				fileWriter = fileutils.StreamingFileWriter{
					Filename: "/directory-that-does-not-exist/somefile",
				}
			})

			It("returns an error", func() {
				err := fileWriter.WriteStream(reader)
				Expect(err).To(MatchError(ContainSubstring("no such file or directory")))
			})

			It("does not consume data from the reader", func() {
				err := fileWriter.WriteStream(reader)

				Expect(err).To(HaveOccurred())
				Expect(int(reader.Len())).To(Equal(len("some-text")))
			})
		})

		Context("When the file can be created", func() {
			BeforeEach(func() {
				var err error
				tmpFile, err = ioutil.TempFile("", "fileutils-test")
				Expect(err).NotTo(HaveOccurred())

				fileWriter = fileutils.StreamingFileWriter{
					Filename: tmpFile.Name(),
				}
			})

			AfterEach(func() {
				if tmpFile != nil {
					os.Remove(tmpFile.Name())
				}
			})

			It("Writes the contents of the provided reader into the file", func() {
				err := fileWriter.WriteStream(reader)
				Expect(err).ToNot(HaveOccurred())

				fileBytes, err := ioutil.ReadFile(tmpFile.Name())
				Expect(err).ToNot(HaveOccurred())
				Expect(fileBytes).To(Equal([]byte("some-text")))
			})

			Context("when copying the file fails", func() {
				It("returns an error", func() {
					errorReader := errorReader{}
					err := fileWriter.WriteStream(errorReader)

					Expect(err).To(MatchError("Error writing reader contents to file"))
				})
			})
		})
	})
})
