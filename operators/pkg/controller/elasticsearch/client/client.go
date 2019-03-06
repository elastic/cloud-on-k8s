// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

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

	"github.com/elastic/k8s-operators/operators/pkg/utils/net"
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"
	"github.com/pkg/errors"
)

// DefaultVotingConfigExclusionsTimeout is the default timeout for setting voting exclusions.
const DefaultVotingConfigExclusionsTimeout = "30s"

// UserAuth is authentication information for the Elasticsearch client.
type UserAuth struct {
	Name     string
	Password string
}

// Role represents an Elasticsearch role.
type Role struct {
	Cluster []string `json:"cluster,omitempty"`
	/*Indices []struct {
		Names      []string `json:"names,omitempty"`
		Privileges []string `json:",omitempty"`
	} `json:"indices,omitempty"`
	Applications []struct {
		Application string   `json:"application"`
		Privileges  []string `json:"privileges"`
		Resources   []string `json:"resources,omitempty"`
	} `json:"applications,omitempty"`
	RunAs    []string `json:"run_as,omitempty"`
	Metadata *struct {
		Reserved bool `json:"_reserved"`
	} `json:"metadata,omitempty"`
	TransientMetadata *struct {
		Enabled bool `json:"enabled"`
	} `json:"transient_metadata,omitempty"`*/
}

// Client captures the information needed to interact with an Elasticsearch cluster via HTTP
type Client struct {
	User     UserAuth
	HTTP     *http.Client
	Endpoint string
	caCerts  []*x509.Certificate
}

// NewElasticsearchClient creates a new client for the target cluster.
//
// If dialer is not nil, it will be used to create new TCP connections
func NewElasticsearchClient(dialer net.Dialer, esURL string, esUser UserAuth, caCerts []*x509.Certificate) *Client {
	certPool := x509.NewCertPool()
	for _, c := range caCerts {
		certPool.AddCert(c)
	}

	transportConfig := http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: certPool,
		},
	}

	// use the custom dialer if provided
	if dialer != nil {
		transportConfig.DialContext = dialer.DialContext
	}

	return &Client{
		Endpoint: esURL,
		User:     esUser,
		caCerts:  caCerts,
		HTTP: &http.Client{
			Transport: &transportConfig,
		},
	}
}

// Equal returns true if c2 can be considered the same as c
func (c *Client) Equal(c2 *Client) bool {
	// handle nil case
	if c2 == nil && c != nil {
		return false
	}
	// compare ca certs
	if len(c.caCerts) != len(c2.caCerts) {
		return false
	}
	for i := range c.caCerts {
		if !c.caCerts[i].Equal(c2.caCerts[i]) {
			return false
		}
	}
	// compare endpoint and user creds
	return c.Endpoint == c2.Endpoint &&
		c.User == c2.User
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

	if c.User != (UserAuth{}) {
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

	request, err := http.NewRequest(method, stringsutil.Concat(c.Endpoint, pathWithQuery), body)
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

// ReloadSecureSettings will decrypt and re-read the entire keystore, on every cluster node,
// but only the reloadable secure settings will be applied
func (c *Client) ReloadSecureSettings(ctx context.Context) error {
	return c.post(ctx, "/_nodes/reload_secure_settings", nil, nil)
}

// GetNodes calls the _nodes api to return a map(nodeName -> Node)
func (c *Client) GetNodes(ctx context.Context) (Nodes, error) {
	var nodes Nodes
	// restrict call to basic node info only
	return nodes, c.get(ctx, "/_nodes/_all/jvm,settings", &nodes)
}

// GetLicense returns the currently applied license. Can be empty.
func (c *Client) GetLicense(ctx context.Context) (License, error) {
	var license LicenseResponse
	return license.License, c.get(ctx, "/_xpack/license", &license)
}

// UpdateLicense attempts to update cluster license with the given licenses.
func (c *Client) UpdateLicense(ctx context.Context, licenses LicenseUpdateRequest) (LicenseUpdateResponse, error) {
	var response LicenseUpdateResponse
	return response, c.post(ctx, "/_xpack/license", licenses, &response)
}
