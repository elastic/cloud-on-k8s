package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
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
}

// NewElasticsearchClient creates a new client bound to the given stack instance.
func NewElasticsearchClient(esURL string, esUser User, caPool *x509.CertPool) *Client {
	return &Client{
		Endpoint: esURL,
		User:     esUser,
		HTTP: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: caPool,
					// TODO: we can do better.
					InsecureSkipVerify: true,
				},
			},
		},
	}
}

// APIError is a non 2xx response from the Elasticsearch API
type APIError struct {
	response *http.Response
}

// Error() implements the error interface.
func (e *APIError) Error() string {
	return e.response.Status
}

// IsNotFound checks whether the error was a HTTP 404 error.
func IsNotFound(err error) bool {
	switch err := err.(type) {
	case *APIError:
		return err.response.StatusCode == http.StatusNotFound
	default:
		return false
	}
}

func checkError(response *http.Response) error {
	if response == nil {
		return fmt.Errorf("received a <nil> response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &APIError{
			response: response,
		}
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

func (c *Client) marshalAndRequest(context context.Context, payload interface{}, newRequest func(at io.Reader) (*http.Request, error)) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	request, err := newRequest(bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	_, err = c.makeRequest(context, request)
	return err

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

	return c.marshalAndRequest(context, allocationSetting, func(body io.Reader) (*http.Request, error) {
		return http.NewRequest(http.MethodPut, fmt.Sprintf("%s/_cluster/settings", c.Endpoint), body)
	})
}

// GetClusterHealth calls the _cluster/health api.
func (c *Client) GetClusterHealth(context context.Context) (Health, error) {
	var result Health
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/_cluster/health", c.Endpoint), nil)
	if err != nil {
		return result, err
	}
	return result, c.makeRequestAndUnmarshal(context, request, &result)
}

// GetSnapshotRepository retrieves the currently configured snapshot repository with the given name.
func (c *Client) GetSnapshotRepository(ctx context.Context, name string) (SnapshotRepository, error) {
	var result map[string]SnapshotRepository
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/_snapshot/%s", c.Endpoint, name), nil)
	if err != nil {
		return result[name], err
	}
	return result[name], c.makeRequestAndUnmarshal(ctx, request, &result)
}

// DeleteSnapshotRepository tries to delete the snapshot repository identified by name.
func (c *Client) DeleteSnapshotRepository(ctx context.Context, name string) error {
	request, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/_snapshot/%s", c.Endpoint, name), nil)
	if err != nil {
		return err
	}
	_, err = c.makeRequest(ctx, request)
	return err
}

// UpsertSnapshotRepository inserts or updates the given snapshot repository
func (c *Client) UpsertSnapshotRepository(context context.Context, name string, repository SnapshotRepository) error {
	return c.marshalAndRequest(context, repository, func(body io.Reader) (*http.Request, error) {
		return http.NewRequest(http.MethodPut, fmt.Sprintf("%s/_snapshot/%s", c.Endpoint, name), body)
	})
}

// GetAllSnapshots returns a list of all snapshots for the given repository.
func (c *Client) GetAllSnapshots(ctx context.Context, repo string) (SnapshotsList, error) {
	var result SnapshotsList
	request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/_snapshot/%s/_all", c.Endpoint, repo), nil)
	if err != nil {
		return result, err
	}
	return result, c.makeRequestAndUnmarshal(ctx, request, &result)
}

// TakeSnapshot takes a new cluster snapshot with the given name into the given repository.
func (c *Client) TakeSnapshot(ctx context.Context, repo string, snapshot string) error {
	request, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/_snapshot/%s/%s", c.Endpoint, repo, snapshot), nil)
	if err != nil {
		return err
	}
	_, err = c.makeRequest(ctx, request)
	return err
}

// DeleteSnapshot deletes the given snapshot from the given repository.
func (c *Client) DeleteSnapshot(ctx context.Context, repo string, snapshot string) error {
	request, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/_snapshot/%s/%s", c.Endpoint, repo, snapshot), nil)
	if err != nil {
		return err
	}
	_, err = c.makeRequest(ctx, request)
	return err
}

func (c *Client) SetMinimumMasterNodes(ctx context.Context, n int) error {
	zenSettings := DiscoveryZenSettings{
		Transient:  DiscoveryZen{MinimumMasterNodes: n},
		Persistent: DiscoveryZen{MinimumMasterNodes: n},
	}
	return c.marshalAndRequest(ctx, zenSettings, func(body io.Reader) (*http.Request, error) {
		return http.NewRequest(http.MethodPut, fmt.Sprintf("%s/_cluster/settings", c.Endpoint), body)
	})
}
