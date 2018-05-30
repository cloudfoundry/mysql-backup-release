package main_test

import (
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Main", func() {
	var session *gexec.Session

	Describe("Flags", func() {

		BeforeEach(func() {
			command := exec.Command(pathToMainBinary, "-help")
			var err error
			session, err = gexec.Start(command, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			session.Kill()
		})

		It("has the correct help flags", func() {
			expectedFlags := []string{
				"-logLevel",
			}

			Eventually(session).Should(gexec.Exit())

			contents := session.Err.Contents()
			for _, flag := range expectedFlags {
				Expect(contents).Should(ContainSubstring(flag))
			}
		})
	})
})
