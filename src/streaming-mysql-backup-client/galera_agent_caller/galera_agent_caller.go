package galera_agent_caller

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/cloudfoundry/streaming-mysql-backup-client/config"
)

type GaleraAgentCallerInterface interface {
	WsrepLocalIndex(string) (int, error)
}

type GaleraAgentCaller struct {
	galeraAgentPort int
}

func DefaultGaleraAgentCaller(galeraAgentPort int) GaleraAgentCallerInterface {
	return &GaleraAgentCaller{
		galeraAgentPort: galeraAgentPort,
	}
}

type status struct {
	WsrepLocalIndex int  `json:"wsrep_local_index"`
	Healthy         bool `json:"healthy"`
}

func (galeraAgentCaller *GaleraAgentCaller) WsrepLocalIndex(ip string) (int, error) {
	httpClient := &http.Client{}
	url := fmt.Sprintf("http://%s:%d/api/v1/status", ip, galeraAgentCaller.galeraAgentPort)

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return -1, err
	}

	resp, err := httpClient.Do(request)
	if err != nil {
		return -1, err
	}
	if resp.StatusCode != http.StatusOK {
		return -1, errors.New("Error response from node")
	}

	var nodeStatus status
	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return -1, err
	}

	err = json.Unmarshal(body, &nodeStatus)
	if err != nil {
		return -1, err
	}
	if !nodeStatus.Healthy {
		return -1, errors.New("Node is not healthy")
	}
	return nodeStatus.WsrepLocalIndex, nil
}

func NewGaleraAgentHTTPClient(config config.BackendTLS) *http.Client {
	client := &http.Client{}

	if config.Enabled {
		certPool := x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM([]byte(config.CA)); !ok {
			// TODO: should we handle the failure parsing a CA?
		}

		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    certPool,
				ServerName: config.ServerName,
			},
		}
	}

	return client
}
