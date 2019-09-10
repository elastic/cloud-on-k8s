// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	fixtures "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/elastic/cloud-on-k8s/pkg/dev/portforward"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRoutingTable(t *testing.T) {

	tests := []struct {
		name string
		args string
		want []Shard
	}{
		{
			name: "Can parse populated routing table",
			args: fixtures.ClusterStateSample,
			want: []Shard{
				{Index: "sample-data-2", Shard: 0, Primary: true, State: STARTED, Node: "stack-sample-es-lkrjf7224s"},
				{Index: "sample-data-2", Shard: 1, Primary: false, State: STARTED, Node: "stack-sample-es-4fxm76vnwj"},
				{Index: "sample-data-2", Shard: 2, Primary: true, State: UNASSIGNED, Node: ""},
			},
		},
		{
			name: "Can parse an empty routing table",
			args: fixtures.EmptyClusterStateSample,
			want: []Shard{},
		},
	}

	for _, tt := range tests {
		var clusterState ClusterState
		b := []byte(tt.args)
		err := json.Unmarshal(b, &clusterState)
		if err != nil {
			t.Error(err)
		}
		shards := clusterState.GetShards()
		assert.True(t, len(shards) == len(tt.want))
		sort.SliceStable(shards, func(i, j int) bool {
			return shards[i].Shard < shards[j].Shard
		})
		for i := range shards {
			assert.EqualValues(t, tt.want[i], shards[i])
		}

	}

}

func TestShardsByNode(t *testing.T) {

	tests := []struct {
		name string
		args string
		want map[string][]Shard
	}{
		{
			name: "Can parse populated routing table",
			args: fixtures.ClusterStateSample,
			want: map[string][]Shard{
				"stack-sample-es-lkrjf7224s": {{Index: "sample-data-2", Shard: 0, Primary: true, State: STARTED, Node: "4cHWfQAwQQKTvKV1vrtbDQ"}},
				"stack-sample-es-4fxm76vnwj": {{Index: "sample-data-2", Shard: 1, Primary: false, State: STARTED, Node: "SaGT6YMJQyS409ZhonOLhQ"}},
			},
		},
	}

	for _, tt := range tests {
		var clusterState ClusterState
		b := []byte(tt.args)
		err := json.Unmarshal(b, &clusterState)
		if err != nil {
			t.Error(err)
		}
		shardsByNode := clusterState.GetShardsByNode()
		assert.True(t, len(shardsByNode) == len(tt.want))
		for node, shards := range shardsByNode {
			expected, ok := tt.want[node]
			assert.True(t, ok)
			assert.EqualValues(t, expected, shards)
		}

	}

}

func errorResponses(statusCodes []int) RoundTripFunc {
	i := 0
	return func(req *http.Request) *http.Response {
		nextCode := statusCodes[i%len(statusCodes)]
		i++
		return &http.Response{
			StatusCode: nextCode,
			Body:       nil,
			Header:     make(http.Header),
			Request:    req,
		}
	}

}

func requestAssertion(test func(req *http.Request)) RoundTripFunc {
	return func(req *http.Request) *http.Response {
		test(req)
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(`{}`)),
			Header:     make(http.Header),
			Request:    req,
		}
	}
}

func TestClientErrorHandling(t *testing.T) {
	// 303 would lead to a redirect to another error response if we would also set the Location header
	codes := []int{100, 303, 400, 404, 500}
	testClient := NewMockClient(version.MustParse("6.7.0"), errorResponses(codes))
	requests := []func() (string, error){
		func() (string, error) {
			_, err := testClient.GetClusterState(context.Background())
			return "GetClusterState", err
		},
		func() (string, error) {
			return "ExcludeFromShardAllocation", testClient.ExcludeFromShardAllocation(context.Background(), "")
		},
	}

	for range codes {
		for _, f := range requests {
			name, err := f()
			assert.Error(t, err, fmt.Sprintf("%s should return an error for anything not 2xx", name))
		}
	}

}

func TestClientUsesJsonContentType(t *testing.T) {
	testClient := NewMockClient(version.MustParse("6.7.0"), requestAssertion(func(req *http.Request) {
		assert.Equal(t, []string{"application/json; charset=utf-8"}, req.Header["Content-Type"])
	}))

	_, err := testClient.GetClusterState(context.Background())
	assert.NoError(t, err)

	assert.NoError(t, testClient.ExcludeFromShardAllocation(context.Background(), ""))
}

func TestClientSupportsBasicAuth(t *testing.T) {

	type expected struct {
		user        UserAuth
		authPresent bool
	}

	tests := []struct {
		name string
		args UserAuth
		want expected
	}{
		{
			name: "Context with user information should be respected",
			args: UserAuth{Name: "elastic", Password: "changeme"},
			want: expected{
				user:        UserAuth{Name: "elastic", Password: "changeme"},
				authPresent: true,
			},
		},
		{
			name: "Context w/o user information is ok too",
			args: UserAuth{},
			want: expected{
				user:        UserAuth{Name: "", Password: ""},
				authPresent: false,
			},
		},
	}

	for _, tt := range tests {
		testClient := NewMockClientWithUser(version.MustParse("6.7.0"),
			tt.args,
			requestAssertion(func(req *http.Request) {
				username, password, ok := req.BasicAuth()
				assert.Equal(t, tt.want.authPresent, ok)
				assert.Equal(t, tt.want.user.Name, username)
				assert.Equal(t, tt.want.user.Password, password)
			}))

		_, err := testClient.GetClusterState(context.Background())
		assert.NoError(t, err)
		assert.NoError(t, testClient.ExcludeFromShardAllocation(context.Background(), ""))

	}

}

func TestClient_request(t *testing.T) {
	testPath := "/_i_am_an/elasticsearch/endpoint"
	testClient := &baseClient{
		HTTP: &http.Client{
			Transport: requestAssertion(func(req *http.Request) {
				assert.Equal(t, testPath, req.URL.Path)
			}),
		},
		Endpoint: "http://example.com",
	}
	requests := []func() (string, error){
		func() (string, error) {
			return "get", testClient.get(context.Background(), testPath, nil)
		},
		func() (string, error) {
			return "put", testClient.put(context.Background(), testPath, nil, nil)
		},
		func() (string, error) {
			return "delete", testClient.delete(context.Background(), testPath, nil, nil)
		},
	}

	for _, f := range requests {
		name, err := f()
		assert.NoError(t, err, fmt.Sprintf("%s should not return an error", name))
	}
}

func TestAPIError_Error(t *testing.T) {
	type fields struct {
		response *http.Response
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "Elasticsearch JSON error response",
			fields: fields{&http.Response{
				Status: "400 Bad Request",
				Body:   ioutil.NopCloser(bytes.NewBufferString(fixtures.ErrorSample)),
			}},
			want: "400 Bad Request: illegal value can't update [discovery.zen.minimum_master_nodes] from [1] to [6]",
		},
		{
			name: "non-JSON error response",
			fields: fields{&http.Response{
				Status: "500 Internal Server Error",
				Body:   ioutil.NopCloser(bytes.NewBufferString("")),
			}},
			want: "500 Internal Server Error: unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &APIError{
				response: tt.fields.response,
			}
			if got := e.Error(); got != tt.want {
				t.Errorf("APIError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClientGetNodes(t *testing.T) {
	expectedPath := "/_nodes/_all/jvm,settings"
	testClient := NewMockClient(version.MustParse("6.7.0"), func(req *http.Request) *http.Response {
		require.Equal(t, expectedPath, req.URL.Path)
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(fixtures.NodesSample)),
			Header:     make(http.Header),
			Request:    req,
		}
	})
	resp, err := testClient.GetNodes(context.Background())
	require.NoError(t, err)
	require.Equal(t, 3, len(resp.Nodes))
	require.Contains(t, resp.Nodes, "iXqjbgPYThO-6S7reL5_HA")
	require.ElementsMatch(t, []string{"master", "data", "ingest"}, resp.Nodes["iXqjbgPYThO-6S7reL5_HA"].Roles)
	require.Equal(t, 2130051072, resp.Nodes["iXqjbgPYThO-6S7reL5_HA"].JVM.Mem.HeapMaxInBytes)
}

func TestClientGetNodesStats(t *testing.T) {
	expectedPath := "/_nodes/_all/stats/os"
	testClient := NewMockClient(version.MustParse("6.7.0"), func(req *http.Request) *http.Response {
		require.Equal(t, expectedPath, req.URL.Path)
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(fixtures.NodesStatsSample)),
			Header:     make(http.Header),
			Request:    req,
		}
	})
	resp, err := testClient.GetNodesStats(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Nodes))
	require.Contains(t, resp.Nodes, "Rt-o5-ZBQaq-Nkhhy0p7JA")
	require.Equal(t, "3221225472", resp.Nodes["Rt-o5-ZBQaq-Nkhhy0p7JA"].OS.CGroup.Memory.LimitInBytes)
}

func TestGetInfo(t *testing.T) {
	expectedPath := "/"
	testClient := NewMockClient(version.MustParse("6.4.1"), func(req *http.Request) *http.Response {
		require.Equal(t, expectedPath, req.URL.Path)
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(fixtures.InfoSample)),
			Header:     make(http.Header),
			Request:    req,
		}
	})
	info, err := testClient.GetClusterInfo(context.Background())
	require.NoError(t, err)
	require.Equal(t, "af932d24216a4dd69ba47d2fd3214796", info.ClusterName)
	require.Equal(t, "LGA3VblKTNmzP6Q6SWxfkw", info.ClusterUUID)
	require.Equal(t, "6.4.1", info.Version.Number)
}

func TestClient_Equal(t *testing.T) {
	dummyEndpoint := "es-url"
	dummyUser := UserAuth{Name: "user", Password: "password"}
	createCert := func() *x509.Certificate {
		ca, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{})
		require.NoError(t, err)
		return ca.Cert
	}
	dummyCACerts := []*x509.Certificate{createCert()}
	v6 := version.MustParse("6.7.0")
	v7 := version.MustParse("7.0.0")
	x509.NewCertPool()
	tests := []struct {
		name string
		c1   Client
		c2   Client
		want bool
	}{
		{
			name: "c1 and c2 equals",
			c1:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, v6, dummyCACerts),
			c2:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, v6, dummyCACerts),
			want: true,
		},
		{
			name: "c2 nil",
			c1:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, v6, dummyCACerts),
			c2:   nil,
			want: false,
		},
		{
			name: "different endpoint",
			c1:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, v6, dummyCACerts),
			c2:   NewElasticsearchClient(nil, "another-endpoint", dummyUser, v6, dummyCACerts),
			want: false,
		},
		{
			name: "different user",
			c1:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, v6, dummyCACerts),
			c2:   NewElasticsearchClient(nil, dummyEndpoint, UserAuth{Name: "user", Password: "another-password"}, v6, dummyCACerts),
			want: false,
		},
		{
			name: "different CA cert",
			c1:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, v6, dummyCACerts),
			c2:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, v6, []*x509.Certificate{createCert()}),
			want: false,
		},
		{
			name: "different CA certs length",
			c1:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, v6, dummyCACerts),
			c2:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, v6, []*x509.Certificate{createCert(), createCert()}),
			want: false,
		},
		{
			name: "different dialers are not taken into consideration",
			c1:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, v6, dummyCACerts),
			c2:   NewElasticsearchClient(portforward.NewForwardingDialer(), dummyEndpoint, dummyUser, v6, dummyCACerts),
			want: true,
		},
		{
			name: "different versions",
			c1:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, v6, dummyCACerts),
			c2:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, v7, dummyCACerts),
			want: false,
		},
		{
			name: "same versions",
			c1:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, v7, dummyCACerts),
			c2:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, v7, dummyCACerts),
			want: true,
		},
		{
			name: "one has a version",
			c1:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, v7, dummyCACerts),
			c2:   NewElasticsearchClient(nil, dummyEndpoint, dummyUser, version.Version{}, dummyCACerts),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.True(t, tt.c1.Equal(tt.c2) == tt.want)
		})
	}
}

func TestClient_UpdateLicense(t *testing.T) {
	tests := []struct {
		expectedPath string
		version      version.Version
	}{
		{
			expectedPath: "/_xpack/license",
			version:      version.MustParse("6.7.0"),
		},
		{
			expectedPath: "/_license",
			version:      version.MustParse("7.0.0"),
		},
	}
	for _, tt := range tests {
		testClient := NewMockClient(tt.version, func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader(fixtures.LicenseUpdateResponseSample)),
				Header:     make(http.Header),
				Request:    req,
			}
		})
		in := LicenseUpdateRequest{
			Licenses: []License{
				{
					UID:                "893361dc-9749-4997-93cb-802e3d7fa4xx",
					Type:               "basic",
					IssueDateInMillis:  0,
					ExpiryDateInMillis: 0,
					MaxNodes:           1,
					IssuedTo:           "unit-test",
					Issuer:             "test-issuer",
					Signature:          "xx",
				},
			},
		}
		got, err := testClient.UpdateLicense(context.Background(), in)
		assert.NoError(t, err)
		assert.Equal(t, true, got.Acknowledged)
		assert.Equal(t, "valid", got.LicenseStatus)
	}

}

func TestClient_GetLicense(t *testing.T) {
	tests := []struct {
		expectedPath string
		version      version.Version
	}{
		{
			expectedPath: "/_xpack/license",
			version:      version.MustParse("6.7.0"),
		},
		{
			expectedPath: "/_license",
			version:      version.MustParse("7.0.0"),
		},
	}

	for _, tt := range tests {
		testClient := NewMockClient(tt.version, func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader(fixtures.LicenseGetSample)),
				Header:     make(http.Header),
				Request:    req,
			}
		})
		got, err := testClient.GetLicense(context.Background())
		assert.NoError(t, err)
		assert.Equal(t, "893361dc-9749-4997-93cb-802e3d7fa4xx", got.UID)
		assert.Equal(t, "platinum", got.Type)
		assert.EqualValues(t, time.Unix(0, 1548115200000*int64(time.Millisecond)).UTC(), *got.IssueDate)
		assert.Equal(t, int64(1548115200000), got.IssueDateInMillis)
		assert.EqualValues(t, time.Unix(0, 1561247999999*int64(time.Millisecond)).UTC(), *got.ExpiryDate)
		assert.Equal(t, int64(1561247999999), got.ExpiryDateInMillis)
		assert.Equal(t, 100, got.MaxNodes)
		assert.Equal(t, "issuer", got.Issuer)
		assert.Equal(t, int64(1548115200000), got.StartDateInMillis)
	}
}

func TestClient_AddVotingConfigExclusions(t *testing.T) {
	tests := []struct {
		expectedPath string
		version      version.Version
		wantErr      bool
	}{
		{
			expectedPath: "",
			version:      version.MustParse("6.7.0"),
			wantErr:      true,
		},
		{
			expectedPath: "/_cluster/voting_config_exclusions/a,b",
			version:      version.MustParse("7.0.0"),
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		client := NewMockClient(tt.version, func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader("")),
			}
		})
		err := client.AddVotingConfigExclusions(context.Background(), []string{"a", "b"}, "")
		if (err != nil) != tt.wantErr {
			t.Errorf("Client.AddVotingConfigExlusions() error = %v, wantErr %v", err, tt.wantErr)
		}
	}
}

func TestClient_DeleteVotingConfigExclusions(t *testing.T) {
	tests := []struct {
		expectedPath string
		version      version.Version
		wantErr      bool
	}{
		{
			expectedPath: "",
			version:      version.MustParse("6.7.0"),
			wantErr:      true,
		},
		{
			expectedPath: "/_cluster/voting_config_exclusions",
			version:      version.MustParse("7.0.0"),
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		client := NewMockClient(tt.version, func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader("")),
			}
		})
		err := client.DeleteVotingConfigExclusions(context.Background(), false)
		if (err != nil) != tt.wantErr {
			t.Errorf("Client.DeleteVotingConfigExclusions() error = %v, wantErr %v", err, tt.wantErr)
		}
	}
}

func TestClient_SetMinimumMasterNodes(t *testing.T) {
	tests := []struct {
		name         string
		expectedPath string
		version      version.Version
		wantErr      bool
	}{
		{
			name:         "mininum master nodes is essential in v6",
			expectedPath: "/_cluster/settings",
			version:      version.MustParse("6.7.0"),
			wantErr:      false,
		},
		{
			name:         "in v7 it is still supported for bwc but devoid of meaning",
			expectedPath: "/_cluster/settings",
			version:      version.MustParse("7.0.0"),
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		client := NewMockClient(tt.version, func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader("")),
			}
		})
		err := client.SetMinimumMasterNodes(context.Background(), 1)
		if (err != nil) != tt.wantErr {
			t.Errorf("Client.SetMinimumMasterNodes() error = %v, wantErr %v", err, tt.wantErr)
		}
	}
}
