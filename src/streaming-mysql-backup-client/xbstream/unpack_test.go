package xbstream

import (
	"bytes"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("xbstream.Unpacker.WriteStream()", func() {
	var (
		tempDir string
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "unxbstream")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	It("unpacks the provided xbstream archive into the destination directory", func() {
		xbstreamUnpacker := NewUnpacker(tempDir)

		xbStreamFile, err := os.Open("fixtures/xbstream.xb")
		Expect(err).ToNot(HaveOccurred())
		defer func(xbStreamFile *os.File) {
			_ = xbStreamFile.Close()
		}(xbStreamFile)

		err = xbstreamUnpacker.WriteStream(xbStreamFile)
		Expect(err).ToNot(HaveOccurred())

		Expect(fmt.Sprintf("%s/xbstreamfile/hello.txt", tempDir)).To(BeAnExistingFile())
	})

	Context("when the xbstream command fails", func() {
		It("returns an error with the combined output of the xbstream command", func() {
			xbstreamUnpacker := NewUnpacker("/some/fake/directory")

			err := xbstreamUnpacker.WriteStream(&bytes.Buffer{})
			Expect(err).To(MatchError(ContainSubstring("xbstream: Can't change dir to '/some/fake/directory' (OS errno 2 - No such file or directory)")))
		})
	})
})
