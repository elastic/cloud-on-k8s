package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// User captures Elasticsearch user credentials.
type User struct {
	Name     string
	Password string
}

// Client captures the information needed to interact with an Elasticsearch cluster via HTTP
type Client struct {
	User     User
	HTTP     *http.Client
	Endpoint string
} //TODO TLS

func checkError(response *http.Response) error {
	if response == nil {
		return fmt.Errorf("received a <nil> response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s, %v", response.Request.URL, response.Status, response.Header)
	}
	return nil
}

func parseRoutingTable(raw ClusterState) ([]Shard, error) {
	var result []Shard
	for _, index := range raw.RoutingTable.Indices {
		for _, shards := range index.Shards {
			for _, shard := range shards {
				shard.Node = raw.Nodes[shard.Node].Name
				result = append(result, shard)
			}
		}
	}
	return result, nil

}

func (c *Client) makeRequest(context context.Context, request *http.Request) (*http.Response, error) {

	withContext := request.WithContext(context)
	withContext.Header.Set("Content-Type", "application/json; charset=utf-8")

	if c.User != (User{}) {
		withContext.SetBasicAuth(c.User.Name, c.User.Password)
	}

	response, err := c.HTTP.Do(withContext)
	if err != nil {
		return response, err
	}
	err = checkError(response)
	return response, err
}

func (c *Client) makeRequestAndUnmarshal(context context.Context, request *http.Request, out interface{}) error {
	resp, err := c.makeRequest(context, request)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(out)
	if err != nil {
		return err
	}
	return nil
}

// GetShards reads all shards from cluster state,
// similar to what _cat/shards does but it is consistent in
// its output.
func (c *Client) GetShards(context context.Context) ([]Shard, error) {
	result := []Shard{}
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/_cluster/state", c.Endpoint), nil)
	if err != nil {
		return result, err
	}

	var clusterState ClusterState
	err = c.makeRequestAndUnmarshal(context, req, &clusterState)
	if err != nil {
		return result, err
	}
	return parseRoutingTable(clusterState)
}

// ExcludeFromShardAllocation takes a comma-separated string of node names and
// configures transient allocation excludes for the given nodes.
func (c *Client) ExcludeFromShardAllocation(context context.Context, nodes string) error {
	allocationSetting := ClusterRoutingAllocation{AllocationSettings{ExcludeName: nodes, Enable: "all"}}
	body, err := json.Marshal(allocationSetting)
	if err != nil {
		return err
	}

	request, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/_cluster/settings", c.Endpoint), bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	_, err = c.makeRequest(context, request)
	return err
}

func (c *Client) GetClusterHealth(context context.Context) (Health, error) {
	var result Health
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/_cluster/health", c.Endpoint), nil)
	if err != nil {
		return result, err
	}
	return result, c.makeRequestAndUnmarshal(context, request, &result)
}
