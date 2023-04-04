package commandexecutor_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCommandExecutor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Command Executor Suite")
}
