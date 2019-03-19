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
	"reflect"
	"strconv"
	"strings"

	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/utils/net"
	"github.com/elastic/k8s-operators/operators/pkg/utils/stringsutil"

	"github.com/pkg/errors"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var (
	log = logf.Log.WithName("client")
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

// // Interface captures the information needed to interact with an Elasticsearch cluster via HTTP
type Interface interface {
	Equal(other Interface) bool
	// GetClusterState returns the current cluster state
	GetClusterInfo(ctx context.Context) (Info, error)
	GetClusterState(ctx context.Context) (ClusterState, error)
	// ExcludeFromShardAllocation takes a comma-separated string of node names and
	// configures transient allocation excludes for the given nodes.
	ExcludeFromShardAllocation(ctx context.Context, nodes string) error
	// GetClusterHealth calls the _cluster/health api.
	GetClusterHealth(ctx context.Context) (Health, error)
	// GetSnapshotRepository retrieves the currently configured snapshot repository with the given name.
	GetSnapshotRepository(ctx context.Context, name string) (SnapshotRepository, error)
	// DeleteSnapshotRepository tries to delete the snapshot repository identified by name.
	DeleteSnapshotRepository(ctx context.Context, name string) error
	// UpsertSnapshotRepository inserts or updates the given snapshot repository
	UpsertSnapshotRepository(ctx context.Context, name string, repository SnapshotRepository) error
	// GetAllSnapshots returns a list of all snapshots for the given repository.
	GetAllSnapshots(ctx context.Context, repo string) (SnapshotsList, error)
	// TakeSnapshot takes a new cluster snapshot with the given name into the given repository.
	TakeSnapshot(ctx context.Context, repo string, snapshot string) error
	// DeleteSnapshot deletes the given snapshot from the given repository.
	DeleteSnapshot(ctx context.Context, repo string, snapshot string) error
	// SetMinimumMasterNodes sets the transient and persistent setting of the same name in cluster settings.
	SetMinimumMasterNodes(ctx context.Context, n int) error
	// AddVotingConfigExclusions sets the transient and persistent setting of the same name in cluster settings.
	//
	// If timeout is the empty string, the default is used.
	//
	// Introduced in: Elasticsearch 7.0.0
	AddVotingConfigExclusions(ctx context.Context, nodeNames []string, timeout string) error
	// DeleteVotingConfigExclusions sets the transient and persistent setting of the same name in cluster settings.
	//
	// Introduced in: Elasticsearch 7.0.0
	DeleteVotingConfigExclusions(ctx context.Context, waitForRemoval bool) error
	// ReloadSecureSettings will decrypt and re-read the entire keystore, on every cluster node,
	// but only the reloadable secure settings will be applied
	ReloadSecureSettings(ctx context.Context) error
	// GetNodes calls the _nodes api to return a map(nodeName -> Node)
	GetNodes(ctx context.Context) (Nodes, error)
	// GetLicense returns the currently applied license. Can be empty.
	GetLicense(ctx context.Context) (License, error)
	// UpdateLicense attempts to update cluster license with the given licenses.
	UpdateLicense(ctx context.Context, licenses LicenseUpdateRequest) (LicenseUpdateResponse, error)
}

type clientV6 struct {
	User     UserAuth
	HTTP     *http.Client
	Endpoint string
	caCerts  []*x509.Certificate
	version  version.Version
}

// GetClusterInfo get the cluster information at /
func (c *clientV6) GetClusterInfo(ctx context.Context) (Info, error) {
	var info Info
	return info, c.get(ctx, "/", &info)
}

func (c *clientV6) GetClusterState(ctx context.Context) (ClusterState, error) {
	var clusterState ClusterState
	return clusterState, c.get(ctx, "/_cluster/state/dispatcher,master_node,nodes,routing_table", &clusterState)

}

func (c *clientV6) ExcludeFromShardAllocation(ctx context.Context, nodes string) error {
	allocationSetting := ClusterRoutingAllocation{AllocationSettings{ExcludeName: nodes, Enable: "all"}}
	return c.put(ctx, "/_cluster/settings", allocationSetting, nil)
}

func (c *clientV6) GetClusterHealth(ctx context.Context) (Health, error) {
	var result Health
	return result, c.get(ctx, "/_cluster/health", &result)
}

func (c *clientV6) GetSnapshotRepository(ctx context.Context, name string) (SnapshotRepository, error) {
	var result map[string]SnapshotRepository
	return result[name], c.get(ctx, path.Join("/_snapshot", name), &result)

}

func (c *clientV6) DeleteSnapshotRepository(ctx context.Context, name string) error {
	return c.delete(ctx, path.Join("/_snapshot", name), nil, nil)
}

func (c *clientV6) UpsertSnapshotRepository(ctx context.Context, name string, repository SnapshotRepository) error {
	return c.put(ctx, path.Join("/_snapshot", name), repository, nil)
}

func (c *clientV6) GetAllSnapshots(ctx context.Context, repo string) (SnapshotsList, error) {
	var result SnapshotsList
	return result, c.get(ctx, path.Join("/_snapshot", repo, "_all"), &result)

}

func (c *clientV6) TakeSnapshot(ctx context.Context, repo string, snapshot string) error {
	return c.put(ctx, path.Join("/_snapshot", repo, snapshot), nil, nil)
}

func (c *clientV6) DeleteSnapshot(ctx context.Context, repo string, snapshot string) error {
	return c.delete(ctx, path.Join("/_snapshot", repo, snapshot), nil, nil)
}

func (c *clientV6) SetMinimumMasterNodes(ctx context.Context, n int) error {
	zenSettings := DiscoveryZenSettings{
		Transient:  DiscoveryZen{MinimumMasterNodes: n},
		Persistent: DiscoveryZen{MinimumMasterNodes: n},
	}
	return c.put(ctx, "/_cluster/settings", &zenSettings, nil)

}

func (c *clientV6) AddVotingConfigExclusions(ctx context.Context, nodeNames []string, timeout string) error {
	return errors.New("Not supported in Elasticsearch 6.x")
}

func (c *clientV6) DeleteVotingConfigExclusions(ctx context.Context, waitForRemoval bool) error {
	return errors.New("Not supported in Elasticsearch 6.x")
}

func (c *clientV6) ReloadSecureSettings(ctx context.Context) error {
	return c.post(ctx, "/_nodes/reload_secure_settings", nil, nil)
}

func (c *clientV6) GetNodes(ctx context.Context) (Nodes, error) {
	var nodes Nodes
	// restrict call to basic node info only
	return nodes, c.get(ctx, "/_nodes/_all/jvm,settings", &nodes)

}

func (c *clientV6) GetLicense(ctx context.Context) (License, error) {
	var license LicenseResponse
	return license.License, c.get(ctx, "/_xpack/license", &license)

}

func (c *clientV6) UpdateLicense(ctx context.Context, licenses LicenseUpdateRequest) (LicenseUpdateResponse, error) {
	var response LicenseUpdateResponse
	return response, c.post(ctx, "/_xpack/license", licenses, &response)

}

// equal returns true if c2 can be considered the same as c
func (c *clientV6) equal(c2 *clientV6) bool {
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
	if !reflect.DeepEqual(c.version, c2.version) {
		return false
	}
	// compare endpoint and user creds
	return c.Endpoint == c2.Endpoint &&
		c.User == c2.User
}

// Equal returns true if c2 can be considered the same as c
func (c *clientV6) Equal(c2 Interface) bool {
	other, ok := c2.(*clientV6)
	if !ok {
		return false
	}
	return c.equal(other)
}

var _ Interface = &clientV6{}

type clientV7 struct {
	clientV6
}

func (c *clientV7) AddVotingConfigExclusions(ctx context.Context, nodeNames []string, timeout string) error {
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

func (c *clientV7) DeleteVotingConfigExclusions(ctx context.Context, waitForRemoval bool) error {
	path := fmt.Sprintf(
		"/_cluster/voting_config_exclusions?wait_for_removal=%s",
		strconv.FormatBool(waitForRemoval),
	)

	if err := c.delete(ctx, path, nil, nil); err != nil {
		return errors.Wrap(err, "unable to delete /_cluster/voting_config_exclusions")
	}
	return nil
}

func (c *clientV7) SetMinimumMasterNodes(ctx context.Context, n int) error {
	return errors.New("Not supported in Elasticsearch 7.0")
}

func (c *clientV7) GetLicense(ctx context.Context) (License, error) {
	var license LicenseResponse
	return license.License, c.get(ctx, "/_license", &license)

}

func (c *clientV7) UpdateLicense(ctx context.Context, licenses LicenseUpdateRequest) (LicenseUpdateResponse, error) {
	var response LicenseUpdateResponse
	return response, c.post(ctx, "/_license", licenses, &response)

}

// Equal returns true if c2 can be considered the same as c
func (c *clientV7) Equal(c2 Interface) bool {
	other, ok := c2.(*clientV7)
	if !ok {
		return false
	}
	return c.clientV6.equal(&other.clientV6)
}

var _ Interface = &clientV7{}

// NewElasticsearchClient creates a new client for the target cluster.
//
// If dialer is not nil, it will be used to create new TCP connections
func NewElasticsearchClient(dialer net.Dialer, esURL string, esUser UserAuth, v version.Version, caCerts []*x509.Certificate) Interface {
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
	c6 := &clientV6{
		Endpoint: esURL,
		User:     esUser,
		caCerts:  caCerts,
		HTTP: &http.Client{
			Transport: &transportConfig,
		},
		version: v,
	}
	return versioned(c6, v)
}

func versioned(baseClient *clientV6, v version.Version) Interface {
	switch v.Major {
	case 7:
		return &clientV7{
			clientV6: *baseClient,
		}
	default:
		return baseClient
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

func (c *clientV6) doRequest(context context.Context, request *http.Request) (*http.Response, error) {
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

func (c *clientV6) get(ctx context.Context, pathWithQuery string, out interface{}) error {
	return c.request(ctx, http.MethodGet, pathWithQuery, nil, out)
}

func (c *clientV6) put(ctx context.Context, pathWithQuery string, in, out interface{}) error {
	return c.request(ctx, http.MethodPut, pathWithQuery, in, out)
}

func (c *clientV6) post(ctx context.Context, pathWithQuery string, in, out interface{}) error {
	return c.request(ctx, http.MethodPost, pathWithQuery, in, out)
}

func (c *clientV6) delete(ctx context.Context, pathWithQuery string, in, out interface{}) error {
	return c.request(ctx, http.MethodDelete, pathWithQuery, in, out)
}

// request performs a new http request
//
// if requestObj is not nil, it's marshalled as JSON and used as the request body
// if responseObj is not nil, it should be a pointer to an struct. the response body will be unmarshalled from JSON
// into this struct.
func (c *clientV6) request(
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
