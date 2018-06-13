package galera_agent_caller_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"net/http"
	"net/http/httptest"
	"strconv"
	"streaming-mysql-backup-client/galera_agent_caller"
	"strings"
)

var _ = Describe("Galera Agent", func() {
	var (
		test_server *httptest.Server
		handlerFunc func(http.ResponseWriter, *http.Request)
		galeraAgent galera_agent_caller.GaleraAgentCallerInterface
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

		galeraAgent = galera_agent_caller.DefaultGaleraAgentCaller(port)
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

func writeBody(w http.ResponseWriter, bodyContents []byte) {
	w.Write(bodyContents)
	w.(http.Flusher).Flush()
}
