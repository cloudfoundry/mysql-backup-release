package tarpit_test

import (
	"fmt"
	"github.com/cloudfoundry/streaming-mysql-backup-client/tarpit"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"os"
	"runtime"
)

var _ = Describe("NewOSXTarClient", func() {
	It("Uses gtar", func() {
		extractor := tarpit.NewOSXTarClient()

		Expect(extractor.TarCommand).To(Equal("gtar"))
	})
})

var _ = Describe("NewGnuTarClient", func() {
	It("Uses tar", func() {
		extractor := tarpit.NewGnuTarClient()

		Expect(extractor.TarCommand).To(Equal("tar"))
	})
})

var _ = Describe("Untar", func() {

	It("Untars the file", func() {
		//create tmp directory
		tempDir, err := ioutil.TempDir("/tmp", "untar")
		Expect(err).ToNot(HaveOccurred())
		defer os.RemoveAll(tempDir)

		//Provide an actual TAR file
		myfile := "fixtures/backup.tar"

		//Untar the file to tmp
		var extractor *tarpit.TarClient

		switch runtime.GOOS {
		case "darwin":
			extractor = tarpit.NewOSXTarClient()
		case "linux":
			extractor = tarpit.NewGnuTarClient()
		default:
			panic("unrecognized runtime")
		}

		cmd := extractor.Untar(myfile, tempDir)

		_, err = cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred())

		//Verify the untarred contents
		Expect(fmt.Sprintf("%s/tarfile/hello.txt", tempDir)).To(BeAnExistingFile())
	})

	It("Calls untar with the correct flags", func() {
		extractor := tarpit.NewTarClient("something_untar_command")

		cmd := extractor.Untar("tarFile", "/path/to/untar/directory/")
		Expect(cmd.Args).To(Equal([]string{"something_untar_command", "xvif", "tarFile", "--directory=/path/to/untar/directory/"}))
	})
})

var _ = Describe("Tar", func() {
	It("Calls tar with the correct flags", func() {
		extractor := tarpit.NewTarClient("something_tar_command")

		cmd := extractor.Tar("/path/to/input/directory/")
		Expect(cmd.Args).To(Equal([]string{"something_tar_command", "cvf", "-", "-C", "/path/to/input/directory/", "."}))
	})
})
