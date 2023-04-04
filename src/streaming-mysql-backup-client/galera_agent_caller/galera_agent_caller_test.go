package galera_agent_caller_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	. "github.com/cloudfoundry/streaming-mysql-backup-client/galera_agent_caller"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Galera Agent", func() {
	var (
		testServer  *httptest.Server
		handlerFunc func(http.ResponseWriter, *http.Request)
		galeraAgent *GaleraAgentCaller
		addr        string
		port        int
	)

	BeforeEach(func() {
		handlerFunc = func(w http.ResponseWriter, r *http.Request) {
			writeBody(w, []byte(`{"wsrep_local_state":4,"wsrep_local_state_comment":"Synced","wsrep_local_index":42,"healthy":true}`))
		}

		galeraAgent = &GaleraAgentCaller{
			TLSEnabled: false,
		}
	})

	JustBeforeEach(func() {
		handler := http.HandlerFunc(handlerFunc)
		if galeraAgent.TLSEnabled {
			testServer = httptest.NewTLSServer(handler)
		} else {
			testServer = httptest.NewServer(handler)
		}
		addrAndPort := strings.Split(testServer.Listener.Addr().String(), ":")
		addr = addrAndPort[0]
		port, _ = strconv.Atoi(addrAndPort[1])

		galeraAgent.GaleraAgentPort = port
		galeraAgent.HTTPClient = testServer.Client()
	})

	Describe("WsrepLocalIndex", func() {

		It("returns the wsrep local index for that node", func() {
			wsrepIndex, _ := galeraAgent.WsrepLocalIndex(addr)
			Expect(wsrepIndex).To(Equal(42))
		})

		Context("the galera agent server is not up", func() {
			It("returns an error", func() {
				testServer.Close()
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
				Expect(err).To(MatchError("500 Internal Server Error: Error response from node: Bad things happened"))
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

	When("TLS is properly configured", func() {
		BeforeEach(func() {
			galeraAgent.TLSEnabled = true
		})

		It("connects via TLS", func() {
			index, err := galeraAgent.WsrepLocalIndex(addr)
			Expect(err).ToNot(HaveOccurred())
			Expect(index).To(Equal(42))
		})
	})
})

func writeBody(w http.ResponseWriter, bodyContents []byte) {
	w.Write(bodyContents)
	w.(http.Flusher).Flush()
}
