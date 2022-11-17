package xbstream

import (
	"bytes"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	// TODO: what is this supposed to be?
	XbstreamFileFixturePath = "fixtures/xbstream.tar"
)

var _ = Describe("UntarStreamer.WriteStream()", func() {
	var (
		tempDir string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "untarstream")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	It("unxbstreams the bytes it receives into the destination directory", func() {
		untarStreamer := NewUnXbStreamer(tempDir)

		xbStreamFile, err := os.Open(XbstreamFileFixturePath)
		Expect(err).ToNot(HaveOccurred())

		err = untarStreamer.WriteStream(xbStreamFile)
		Expect(err).ToNot(HaveOccurred())

		Expect(fmt.Sprintf("%s/tarfile/hello.txt", tempDir)).To(BeAnExistingFile())
	})

	Context("when the tar command fails", func() {
		It("returns an error with the combined output of the tar command", func() {
			unXbStreamer := NewUnXbStreamer("/some/fake/directory")

			err := unXbStreamer.WriteStream(&bytes.Buffer{})
			Expect(err).To(MatchError(ContainSubstring("exit status 2")))
			Expect(err).To(MatchError(ContainSubstring("This does not look like a tar archive")))
		})
	})
})
