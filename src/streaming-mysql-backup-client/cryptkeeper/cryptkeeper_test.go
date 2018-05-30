package cryptkeeper_test

import (
	"streaming-mysql-backup-client/cryptkeeper"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
)

var _ = Describe("Cryptkeeper", func() {

	var (
		tmpDir string
	)

	BeforeEach(func() {
		var err error
		tmpDir, err = ioutil.TempDir(os.TempDir(), "backup-Cryptkeeper-test")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	It("encrypts and decrypts", func() {

		passphrase := `SeU0r<1~S8c22drmE8)2ltY"5T1&aL`

		dencryptor := cryptkeeper.NewCryptKeeper(passphrase)

		plaintextBuffer := []byte("world")
		thingToEncrypt := bytes.NewReader(plaintextBuffer)

		encryptedFile, err := os.Create(filepath.Join(tmpDir, "encrypted-backup-test"))
		Expect(err).ToNot(HaveOccurred())
		defer encryptedFile.Close()

		dencryptor.Encrypt(thingToEncrypt, encryptedFile)

		encryptedContents, err := ioutil.ReadAll(encryptedFile)
		Expect(err).ToNot(HaveOccurred())
		Expect(string(encryptedContents)).ToNot(ContainSubstring("world"))

		decryptedFile, err := os.Create(filepath.Join(tmpDir, "decrypted-backup-test"))
		Expect(err).ToNot(HaveOccurred())
		defer decryptedFile.Close()

		decryptWithGpg(passphrase, encryptedFile, decryptedFile)

		decryptedContent, err := ioutil.ReadAll(decryptedFile)
		Expect(err).ToNot(HaveOccurred())

		Expect(string(decryptedContent)).To(Equal("world"))
	})
})

func decryptWithGpg(passphrase string, encryptedFile, decryptedFile *os.File) {

	cmd := exec.Command("gpg",
		"--batch", "--yes", "--no-tty", //non-interactive
		"--compress-algo", "zip",
		"--cipher-algo", "AES256",
		"--output", decryptedFile.Name(),
		"--passphrase", passphrase,
		"--decrypt",
		encryptedFile.Name(),
	)

	output, err := cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Errored with output: %s", output))
}
