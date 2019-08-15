package lifecycle_test

import (
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/pivotal/mysql-test-utils/testhelpers"
)

func TestLifecycle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lifecycle Suite")
}

var _ = BeforeSuite(func() {
	log.SetOutput(GinkgoWriter)
	log.SetFlags(0)

	checkForRequiredEnvVars([]string{
		"BOSH_ENVIRONMENT",
		"BOSH_CA_CERT",
		"BOSH_CLIENT",
		"BOSH_CLIENT_SECRET",
		"BOSH_DEPLOYMENT",
	})
})

func checkForRequiredEnvVars(envs []string) {
	var missingEnvs []string

	for _, v := range envs {
		if os.Getenv(v) == "" {
			missingEnvs = append(missingEnvs, v)
		}
	}

	Expect(missingEnvs).To(BeEmpty(), "Missing environment variables: %s", strings.Join(missingEnvs, ", "))
}

var _ = JustAfterEach(func() {
	if CurrentGinkgoTestDescription().Failed {
		fmt.Fprint(GinkgoWriter, testhelpers.TestFailureMessage)
	}
})
