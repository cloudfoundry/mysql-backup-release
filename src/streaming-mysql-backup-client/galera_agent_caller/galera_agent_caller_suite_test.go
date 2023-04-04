package galera_agent_caller_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGaleraAgent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Streaming MySQL Backup Galera Agent Caller Suite")
}
