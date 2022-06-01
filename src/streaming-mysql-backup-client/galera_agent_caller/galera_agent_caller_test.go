package galera_agent_caller_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"github.com/cloudfoundry/streaming-mysql-backup-client/config"
	. "github.com/cloudfoundry/streaming-mysql-backup-client/galera_agent_caller"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Galera Agent", func() {
	var (
		test_server *httptest.Server
		handlerFunc func(http.ResponseWriter, *http.Request)
		galeraAgent GaleraAgentCallerInterface
		addr        string
		port        int
	)

	BeforeEach(func() {
		handlerFunc = func(w http.ResponseWriter, r *http.Request) {
			writeBody(w, []byte(`{"wsrep_local_state":4,"wsrep_local_state_comment":"Synced","wsrep_local_index":1,"healthy":true}`))
		}
	})

	JustBeforeEach(func() {
		handler := http.HandlerFunc(handlerFunc)
		test_server = httptest.NewServer(handler)
		addrAndPort := strings.Split(test_server.Listener.Addr().String(), ":")
		addr = addrAndPort[0]
		port, _ = strconv.Atoi(addrAndPort[1])

		galeraAgent = DefaultGaleraAgentCaller(port)
	})

	Describe("WsrepLocalIndex", func() {

		It("returns the wsrep local index for that node", func() {
			wsrepIndex, _ := galeraAgent.WsrepLocalIndex(addr)
			Expect(wsrepIndex).To(Equal(1))
		})

		Context("the galera agent server is not up", func() {
			It("returns an error", func() {
				test_server.Close()
				_, err := galeraAgent.WsrepLocalIndex(addr)
				Expect(err).To(MatchError(ContainSubstring("connection refused")))
			})
		})

		Context("the galera agent server returns bad data", func() {
			BeforeEach(func() {
				handlerFunc = func(w http.ResponseWriter, r *http.Request) {
					writeBody(w, []byte(`superbaddata`))
				}
			})
			It("returns an error", func() {
				_, err := galeraAgent.WsrepLocalIndex(addr)
				Expect(err).To(MatchError(ContainSubstring("invalid character")))
			})
		})

		Context("the galera agent server returns a 500 response", func() {
			BeforeEach(func() {
				handlerFunc = func(w http.ResponseWriter, r *http.Request) {
					http.Error(w, "Bad things happened", http.StatusInternalServerError)
				}
			})
			It("returns an error", func() {
				_, err := galeraAgent.WsrepLocalIndex(addr)
				Expect(err).To(MatchError(ContainSubstring("Error response from node")))
			})
		})

		Context("when the node is unhealthy", func() {
			BeforeEach(func() {
				handlerFunc = func(w http.ResponseWriter, r *http.Request) {
					writeBody(w, []byte(`{"wsrep_local_state":3,"wsrep_local_state_comment":"Joined","wsrep_local_index":1,"healthy":false}`))
				}
			})
			It("returns an error", func() {
				_, err := galeraAgent.WsrepLocalIndex(addr)
				Expect(err).To(MatchError(ContainSubstring("Node is not healthy")))
			})
		})
	})

})

var _ = Describe("Galera Agent Client", func() {
	Describe("HTTPClient", func() {
		var (
			httpClient *http.Client
			backendTLS *config.BackendTLS
		)

		BeforeEach(func() {
			// TODO: set this config via OS? like in PXC-release
			backendTLS = &config.BackendTLS{}
			//osArgs := []string{
			//	"galera-init",
			//	"-configPath=fixtures/validConfig.yml",
			//}
			//
			//var err error
			//rootConfig, err = NewConfig(osArgs)
			//Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			httpClient = NewGaleraAgentHTTPClient(*backendTLS)
		})

		It("returns a client", func() {
			Expect(httpClient).ToNot(BeNil())
		})

		When("Galera Agent TLS is not enabled", func() {
			BeforeEach(func() {
				backendTLS.Enabled = false
			})

			It("does not configure a TLSClientConfig", func() {
				Expect(httpClient.Transport).To(BeNil())
			})
		})

		When("Galera Agent TLS is enabled", func() {
			BeforeEach(func() {
				backendTLS.Enabled = true
			})

			It("configures a TLSClientConfig", func() {
				Expect(httpClient.Transport).To(BeAssignableToTypeOf(&http.Transport{}))

				transport := httpClient.Transport.(*http.Transport)

				Expect(transport.TLSClientConfig.ServerName).To(Equal(backendTLS.ServerName))
				Expect(transport.TLSClientConfig.RootCAs).NotTo(BeNil())
			})
		})
	})
})

func writeBody(w http.ResponseWriter, bodyContents []byte) {
	w.Write(bodyContents)
	w.(http.Flusher).Flush()
}
