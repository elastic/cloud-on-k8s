package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type Client struct {
	HTTP     *http.Client
	Endpoint string
} //TODO credentials, TLS

type Shard struct {
	Index  string `json:"index"`
	Shard  string `json:"shard"`
	Prirep string `json:"prirep"`
	State  string `json:"state"`
	Store  string `json:"string"`
	IP     string `json:"ip"`
	Node   string `json:"node"`
}

type TransientSettings struct {
	ExcludeName string `json:"cluster.routing.allocation.exclude._name"`
	Enable      string `json:"cluster.routing.allocation.enable"`
} //TODO awareness settings

type ClusterRoutingAllocation struct {
	Transient TransientSettings `json:"transient"`
}

func checkError(response *http.Response) error {
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New(fmt.Sprintf("%s returned %s, %v", response.Request.URL, response.Status, response.Header))
	}
	return nil
}

func (c *Client) CatShards() ([]Shard, error) {
	result := []Shard{}
	resp, err := c.HTTP.Get(fmt.Sprintf("%s/_cat/shards?format=json", c.Endpoint))
	if err != nil {
		return result, err
	}

	err = checkError(resp)
	if err != nil {
		return result, err
	}

	defer resp.Body.Close()
	return result, json.NewDecoder(resp.Body).Decode(&result)
}

func (c *Client) ExcludeFromShardAllocation(node string) error {
	allocationSetting := ClusterRoutingAllocation{TransientSettings{ExcludeName: node, Enable: "all"}}
	body, err := json.Marshal(allocationSetting)
	if err != nil {
		return err
	}

	request, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/_cluster/settings", c.Endpoint), bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json; charset=utf-8")

	response, err := c.HTTP.Do(request)
	if err != nil {
		return err
	}

	err = checkError(response)
	if err != nil {
		return err
	}
	return nil
}
