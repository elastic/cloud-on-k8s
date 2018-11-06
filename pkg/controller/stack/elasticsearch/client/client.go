package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type userContextKeyType string

const (
	userContextKey userContextKeyType = "api-user"
)

// User captures Elasticsearch user credentials.
type User struct {
	Name     string
	Password string
}

// WithUser adds a user to a context.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, userContextKey, u)
}

// GetUser extracts a user from a context if present.
func GetUser(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userContextKey).(User)
	return u, ok
}

// Client captures the information needed to interact with an Elasticsearch cluster via HTTP
type Client struct {
	Context  context.Context
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

func (c *Client) makeRequest(request *http.Request) (*http.Response, error) {

	withContext := request.WithContext(c.Context)
	withContext.Header.Set("Content-Type", "application/json; charset=utf-8")

	user, ok := GetUser(withContext.Context())
	if ok {
		withContext.SetBasicAuth(user.Name, user.Password)
	}



	response, err := c.HTTP.Do(withContext)
	if err != nil {
		return response, err
	}
	err = checkError(response)
	return response, err
}

// GetShards reads all shards from cluster state,
// similar to what _cat/shards does but it is consistent in
// its output.
func (c *Client) GetShards() ([]Shard, error) {
	result := []Shard{}
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/_cluster/state", c.Endpoint), nil)
	if err != nil {
		return result, err
	}

	resp, err := c.makeRequest(req)
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

	_, err = c.makeRequest(request)
	return err
}
