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
	"path"
	"strconv"
	"strings"

	"github.com/elastic/stack-operators/stack-operator/pkg/controller/common"
	"github.com/elastic/stack-operators/stack-operator/pkg/utils/net"
	"github.com/pkg/errors"
)

// DefaultVotingConfigExclusionsTimeout is the default timeout for setting voting exclusions.
const DefaultVotingConfigExclusionsTimeout = "30s"

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
//
// If dialer is not nil, it will be used to create new TCP connections
func NewElasticsearchClient(dialer net.Dialer, esURL string, esUser User, caPool *x509.CertPool) *Client {
	transportConfig := http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: caPool,
		},
	}

	// use the custom dialer if provided
	if dialer != nil {
		transportConfig.DialContext = dialer.DialContext
	}

	return &Client{
		Endpoint: esURL,
		User:     esUser,
		HTTP: &http.Client{
			Transport: &transportConfig,
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
	reason := "unknown"
	// Elasticsearch has a detailed error message in the response body
	var errMsg ErrorResponse
	err := json.NewDecoder(e.response.Body).Decode(&errMsg)
	if err == nil {
		reason = errMsg.Error.Reason
	}
	return fmt.Sprintf("%s: %s", e.response.Status, reason)
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
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &APIError{
			response: response,
		}
	}
	return nil
}

func (c *Client) doRequest(context context.Context, request *http.Request) (*http.Response, error) {
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

func (c *Client) get(ctx context.Context, pathWithQuery string, out interface{}) error {
	return c.request(ctx, http.MethodGet, pathWithQuery, nil, out)
}

func (c *Client) put(ctx context.Context, pathWithQuery string, in, out interface{}) error {
	return c.request(ctx, http.MethodPut, pathWithQuery, in, out)
}

func (c *Client) post(ctx context.Context, pathWithQuery string, in, out interface{}) error {
	return c.request(ctx, http.MethodPost, pathWithQuery, in, out)
}

func (c *Client) delete(ctx context.Context, pathWithQuery string, in, out interface{}) error {
	return c.request(ctx, http.MethodDelete, pathWithQuery, in, out)
}

// request performs a new http request
//
// if requestObj is not nil, it's marshalled as JSON and used as the request body
// if responseObj is not nil, it should be a pointer to an struct. the response body will be unmarshalled from JSON
// into this struct.
func (c *Client) request(
	ctx context.Context,
	method string,
	pathWithQuery string,
	requestObj,
	responseObj interface{},
) error {
	var body io.Reader = http.NoBody
	if requestObj != nil {
		outData, err := json.Marshal(requestObj)
		if err != nil {
			return err
		}
		body = bytes.NewBuffer(outData)
	}

	request, err := http.NewRequest(method, common.Concat(c.Endpoint, pathWithQuery), body)
	if err != nil {
		return err
	}

	resp, err := c.doRequest(ctx, request)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if responseObj != nil {
		if err := json.NewDecoder(resp.Body).Decode(responseObj); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) GetClusterInfo(ctx context.Context) (Info, error) {
	var info Info
	return info, c.get(ctx, "/", &info)
}

// GetClusterState returns the current cluster state
func (c *Client) GetClusterState(ctx context.Context) (ClusterState, error) {
	var clusterState ClusterState
	return clusterState, c.get(ctx, "/_cluster/state/version,master_node,nodes,routing_table", &clusterState)
}

// ExcludeFromShardAllocation takes a comma-separated string of node names and
// configures transient allocation excludes for the given nodes.
func (c *Client) ExcludeFromShardAllocation(ctx context.Context, nodes string) error {
	allocationSetting := ClusterRoutingAllocation{AllocationSettings{ExcludeName: nodes, Enable: "all"}}
	return c.put(ctx, "/_cluster/settings", allocationSetting, nil)
}

// GetClusterHealth calls the _cluster/health api.
func (c *Client) GetClusterHealth(ctx context.Context) (Health, error) {
	var result Health
	return result, c.get(ctx, "/_cluster/health", &result)
}

// GetSnapshotRepository retrieves the currently configured snapshot repository with the given name.
func (c *Client) GetSnapshotRepository(ctx context.Context, name string) (SnapshotRepository, error) {
	var result map[string]SnapshotRepository
	return result[name], c.get(ctx, path.Join("/_snapshot", name), &result)
}

// DeleteSnapshotRepository tries to delete the snapshot repository identified by name.
func (c *Client) DeleteSnapshotRepository(ctx context.Context, name string) error {
	return c.delete(ctx, path.Join("/_snapshot", name), nil, nil)
}

// UpsertSnapshotRepository inserts or updates the given snapshot repository
func (c *Client) UpsertSnapshotRepository(context context.Context, name string, repository SnapshotRepository) error {
	return c.put(context, path.Join("/_snapshot", name), repository, nil)
}

// GetAllSnapshots returns a list of all snapshots for the given repository.
func (c *Client) GetAllSnapshots(ctx context.Context, repo string) (SnapshotsList, error) {
	var result SnapshotsList
	return result, c.get(ctx, path.Join("/_snapshot", repo, "_all"), &result)
}

// TakeSnapshot takes a new cluster snapshot with the given name into the given repository.
func (c *Client) TakeSnapshot(ctx context.Context, repo string, snapshot string) error {
	return c.put(ctx, path.Join("/_snapshot", repo, snapshot), nil, nil)
}

// DeleteSnapshot deletes the given snapshot from the given repository.
func (c *Client) DeleteSnapshot(ctx context.Context, repo string, snapshot string) error {
	return c.delete(ctx, path.Join("/_snapshot", repo, snapshot), nil, nil)
}

// SetMinimumMasterNodes sets the transient and persistent setting of the same name in cluster settings.
func (c *Client) SetMinimumMasterNodes(ctx context.Context, n int) error {
	zenSettings := DiscoveryZenSettings{
		Transient:  DiscoveryZen{MinimumMasterNodes: n},
		Persistent: DiscoveryZen{MinimumMasterNodes: n},
	}
	return c.put(ctx, "/_cluster/settings", &zenSettings, nil)
}

// AddVotingConfigExclusions sets the transient and persistent setting of the same name in cluster settings.
//
// If timeout is the empty string, the default is used.
//
// Introduced in: Elasticsearch 7.0.0
func (c *Client) AddVotingConfigExclusions(ctx context.Context, nodeNames []string, timeout string) error {
	if timeout == "" {
		timeout = DefaultVotingConfigExclusionsTimeout
	}
	path := fmt.Sprintf(
		"/_cluster/voting_config_exclusions/%s?timeout=%s",
		strings.Join(nodeNames, ","),
		timeout,
	)

	if err := c.post(ctx, path, nil, nil); err != nil {
		return errors.Wrap(err, "unable to add to voting_config_exclusions")
	}
	return nil
}

// DeleteVotingConfigExclusions sets the transient and persistent setting of the same name in cluster settings.
//
// Introduced in: Elasticsearch 7.0.0
func (c *Client) DeleteVotingConfigExclusions(ctx context.Context, waitForRemoval bool) error {
	path := fmt.Sprintf(
		"/_cluster/voting_config_exclusions?wait_for_removal=%s",
		strconv.FormatBool(waitForRemoval),
	)

	if err := c.delete(ctx, path, nil, nil); err != nil {
		return errors.Wrap(err, "unable to delete /_cluster/voting_config_exclusions")
	}
	return nil
}

// GetNodes calls the _nodes api to return a map(nodeName -> Node)
func (c *Client) GetNodes(ctx context.Context) (Nodes, error) {
	var nodes Nodes
	// restrict call to basic node info only
	return nodes, c.get(ctx, "/_nodes/_all/jvm,settings", &nodes)
}
