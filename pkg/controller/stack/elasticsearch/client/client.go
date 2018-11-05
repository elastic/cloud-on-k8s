package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type Client struct {
	HTTP     *http.Client
	Endpoint string
} //TODO credentials, TLS

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

// GetShards reads all shards from cluster state,
// similar to what _cat/shards does but it is consistent in
// its output.
func (c *Client) GetShards() ([]Shard, error) {
	result := []Shard{}
	resp, err := c.HTTP.Get(fmt.Sprintf("%s/_cluster/state", c.Endpoint))
	if err != nil {
		return result, err
	}

	err = checkError(resp)
	if err != nil {
		return result, err
	}

	defer resp.Body.Close()
	var clusterState ClusterState
	err = json.NewDecoder(resp.Body).Decode(&clusterState)
	if err != nil {
		return result, err
	}
	return parseRoutingTable(clusterState)
}

// ExcludeFromShardAllocation takes a comma-separated string of node names and
// configures transient allocation excludes for the given nodes.
func (c *Client) ExcludeFromShardAllocation(nodes string) error {
	allocationSetting := ClusterRoutingAllocation{AllocationSettings{ExcludeName: nodes, Enable: "all"}}
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
