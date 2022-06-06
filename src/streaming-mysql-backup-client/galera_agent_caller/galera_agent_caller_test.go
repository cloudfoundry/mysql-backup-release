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
		backendTLS  config.BackendTLS
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
		backendTLS = config.BackendTLS{Enabled: false}

		galeraAgent = &GaleraAgentCaller{
			GaleraAgentPort:  port,
			GaleraBackendTLS: backendTLS,
		}
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

	var _ = Describe("TLS connectivity", func() {
		BeforeEach(func() {
			handlerFunc = func(w http.ResponseWriter, r *http.Request) {
				writeBody(w, []byte(`{"wsrep_local_state":4,"wsrep_local_state_comment":"Synced","wsrep_local_index":42,"healthy":true}`))
			}
		})
		JustBeforeEach(func() {
			handler := http.HandlerFunc(handlerFunc)
			test_server = httptest.NewTLSServer(handler)
			addrAndPort := strings.Split(test_server.Listener.Addr().String(), ":")
			addr = addrAndPort[0]
			port, _ = strconv.Atoi(addrAndPort[1])
			backendTLS = config.BackendTLS{
				Enabled:            true,
				InsecureSkipVerify: true,
			}

		})
		When("TLS is properly configured", func() {
			It("connects via TLS", func() {
				galeraAgent = &GaleraAgentCaller{
					GaleraAgentPort:  port,
					GaleraBackendTLS: backendTLS,
				}
				index, err := galeraAgent.WsrepLocalIndex(addr)
				Expect(index).To(Equal(42))
				Expect(err).ToNot(HaveOccurred())
			})
		})
		When("the client attempts a non-TLS connection to a TLS back-end", func() {
			BeforeEach(func() {
			})
			It("returns the expected error", func() {
				backendTLS.Enabled = false
				galeraAgent = &GaleraAgentCaller{
					GaleraAgentPort:  port,
					GaleraBackendTLS: backendTLS,
				}
				_, err := galeraAgent.WsrepLocalIndex(addr)
				Expect(err).To(HaveOccurred())
				//TODO this error message needs to be improved
				Expect(err.Error()).To(Equal("Error response from node"))
			})
		})
		When("the server's certificate cannot be authenticated", func() {
			BeforeEach(func() {
				// Since httptest nodes don't support secure TLS connections,
				// this provokes the desired authentication failure.
				//rootConfig.BackendTLS.InsecureSkipVerify = false
			})
			It("returns the expected error", func() {
			})
		})
		When("the server's certificate doesn't contains the expected name", func() {
			BeforeEach(func() {
				//rootConfig.BackendTLS.InsecureSkipVerify = false
				//rootConfig.BackendTLS.ServerName = "incorrectValue.org"
			})
			It("returns the expected error", func() {
			})
		})

	})

	Describe("backendTLS is true", func() {

	})

})

var _ = Describe("Galera Agent Client", func() {
	Describe("HTTPClient", func() {
		var (
			httpClient *http.Client
			backendTLS *config.BackendTLS
		)

		BeforeEach(func() {
			backendTLS = &config.BackendTLS{}
			osArgs := []string{
				"galera-init",
				"-configPath=../client/fixtures/validConfig.yml",
			}

			var err error
			_, err = config.NewConfig(osArgs)
			Expect(err).NotTo(HaveOccurred())
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
