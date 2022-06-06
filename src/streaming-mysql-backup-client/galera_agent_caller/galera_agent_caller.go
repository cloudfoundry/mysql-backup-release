package galera_agent_caller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

type GaleraAgentCallerInterface interface {
	WsrepLocalIndex(string) (int, error)
}

type GaleraAgentCaller struct {
	GaleraAgentPort int
	TLSEnabled      bool
	HTTPClient      *http.Client
}

type status struct {
	WsrepLocalIndex int  `json:"wsrep_local_index"`
	Healthy         bool `json:"healthy"`
}

func (g *GaleraAgentCaller) WsrepLocalIndex(ip string) (int, error) {
	httpClient := g.HTTPClient
	protocol := "http"
	if g.TLSEnabled {
		protocol = "https"
	}
	url := fmt.Sprintf("%s://%s:%d/api/v1/status", protocol, ip, g.GaleraAgentPort)

	resp, err := httpClient.Get(url)
	if err != nil {
		return -1, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return -1, err
	}

	if resp.StatusCode != http.StatusOK {
		return -1, fmt.Errorf("%s: Error response from node: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var nodeStatus status
	err = json.Unmarshal(body, &nodeStatus)
	if err != nil {
		return -1, err
	}
	if !nodeStatus.Healthy {
		return -1, errors.New("Node is not healthy")
	}
	return nodeStatus.WsrepLocalIndex, nil
}
