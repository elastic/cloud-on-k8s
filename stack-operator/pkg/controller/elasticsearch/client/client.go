package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
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

// NewElasticsearchClient creates a new client for the target cluster.
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
	defer e.response.Body.Close()
	body := ""
	// Elasticsearch has a detailed error message in the response body
	bytes, err := ioutil.ReadAll(e.response.Body)
	if err == nil {
		body = string(bytes)
	}
	return fmt.Sprintf("%s %s", e.response.Status, body)
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

// GetClusterState returns the current cluster state
func (c *Client) GetClusterState(context context.Context) (ClusterState, error) {
	var clusterState ClusterState
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/_cluster/state/version,master_node,nodes,routing_table", c.Endpoint), nil)
	if err != nil {
		return clusterState, err
	}
	return clusterState, c.makeRequestAndUnmarshal(context, req, &clusterState)
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

// SetMinimumMasterNodes sets the transient and persistent setting of the same name in cluster settings.
func (c *Client) SetMinimumMasterNodes(ctx context.Context, n int) error {
	zenSettings := DiscoveryZenSettings{
		Transient:  DiscoveryZen{MinimumMasterNodes: n},
		Persistent: DiscoveryZen{MinimumMasterNodes: n},
	}
	return c.marshalAndRequest(ctx, zenSettings, func(body io.Reader) (*http.Request, error) {
		return http.NewRequest(http.MethodPut, fmt.Sprintf("%s/_cluster/settings", c.Endpoint), body)
	})
}
