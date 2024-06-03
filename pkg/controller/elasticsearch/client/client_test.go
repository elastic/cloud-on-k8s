// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	esv1 "github.com/elastic/cloud-on-k8s/v2/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/certificates"
	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
	fixtures "github.com/elastic/cloud-on-k8s/v2/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/elastic/cloud-on-k8s/v2/pkg/dev/portforward"
)

func TestParseShards(t *testing.T) {
	tests := []struct {
		name string
		args string
		want Shards
	}{
		{
			name: "Can parse populated routing table with some relocating shards",
			args: fixtures.RelocatingShards,
			want: Shards{
				Shard{
					Index:    "data-integrity-check",
					Shard:    "0",
					State:    "STARTED",
					NodeName: "test-mutation-less-nodes-sqn9-es-masterdata-0",
				},
				Shard{
					Index:    "data-integrity-check",
					Shard:    "1",
					State:    "RELOCATING",
					NodeName: "test-mutation-less-nodes-sqn9-es-masterdata-1",
				},
				Shard{
					Index:    "data-integrity-check",
					Shard:    "2",
					State:    "RELOCATING",
					NodeName: "test-mutation-less-nodes-sqn9-es-masterdata-2",
				},
				Shard{
					Index:    "data-integrity-check",
					Shard:    "3",
					State:    "UNASSIGNED",
					NodeName: "",
				},
			},
		},
		{
			name: "Can parse an empty routing table",
			args: fixtures.NoShards,
			want: []Shard{},
		},
	}

	for _, tt := range tests {
		var shards Shards
		b := []byte(tt.args)
		err := json.Unmarshal(b, &shards)
		if err != nil {
			t.Error(err)
		}
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
			args: fixtures.SampleShards,
			want: map[string][]Shard{
				"stack-sample-es-lkrjf7224s": {{Index: "sample-data-2", Shard: "0", State: STARTED, NodeName: "stack-sample-es-lkrjf7224s", Type: Primary}},
				"stack-sample-es-4fxm76vnwj": {{Index: "sample-data-2", Shard: "1", State: STARTED, NodeName: "stack-sample-es-4fxm76vnwj", Type: Replica}},
			},
		},
	}

	for _, tt := range tests {
		var shards Shards
		b := []byte(tt.args)
		err := json.Unmarshal(b, &shards)
		if err != nil {
			t.Error(err)
		}
		shardsByNode := shards.GetShardsByNode()
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
			Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
			Header:     make(http.Header),
			Request:    req,
		}
	}
}

func TestClientErrorHandling(t *testing.T) {
	// 303 would lead to a redirect to another error response if we would also set the Location header
	codes := []int{100, 303, 400, 404, 500}
	testClient := NewMockClient(version.MustParse("6.8.0"), errorResponses(codes))
	requests := []func() (string, error){
		func() (string, error) {
			_, err := testClient.GetClusterInfo(context.Background())
			return "GetClusterInfo", err
		},
		func() (string, error) {
			return "SetMinimumMasterNodes", testClient.SetMinimumMasterNodes(context.Background(), 0)
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
	testClient := NewMockClient(version.MustParse("6.8.0"), requestAssertion(func(req *http.Request) {
		assert.Equal(t, []string{"application/json; charset=utf-8"}, req.Header["Content-Type"])
	}))

	_, err := testClient.GetClusterInfo(context.Background())
	assert.NoError(t, err)

	assert.NoError(t, testClient.SetMinimumMasterNodes(context.Background(), 0))
}

func TestClientSupportsBasicAuth(t *testing.T) {
	type expected struct {
		user        BasicAuth
		authPresent bool
	}

	tests := []struct {
		name string
		args BasicAuth
		want expected
	}{
		{
			name: "Context with user information should be respected",
			args: BasicAuth{Name: "elastic", Password: "changeme"},
			want: expected{
				user:        BasicAuth{Name: "elastic", Password: "changeme"},
				authPresent: true,
			},
		},
		{
			name: "Context w/o user information is ok too",
			args: BasicAuth{},
			want: expected{
				user:        BasicAuth{Name: "", Password: ""},
				authPresent: false,
			},
		},
	}

	for _, tt := range tests {
		testClient := NewMockClientWithUser(version.MustParse("6.8.0"),
			tt.args,
			requestAssertion(func(req *http.Request) {
				username, password, ok := req.BasicAuth()
				assert.Equal(t, tt.want.authPresent, ok)
				assert.Equal(t, tt.want.user.Name, username)
				assert.Equal(t, tt.want.user.Password, password)
			}))

		_, err := testClient.GetClusterInfo(context.Background())
		assert.NoError(t, err)
		assert.NoError(t, testClient.SetMinimumMasterNodes(context.Background(), 0))
	}
}

func TestClient_request(t *testing.T) {
	testPath := "/_i_am_an/elasticsearch/endpoint"
	testClient := &baseClient{
		HTTP: &http.Client{
			Transport: requestAssertion(func(req *http.Request) {
				assert.Equal(t, testPath, req.URL.Path)
				assert.Equal(t, "cloud", req.Header.Get("x-elastic-product-origin"))
			}),
		},
		URLProvider: NewStaticURLProvider("http://example.com"),
	}
	requests := []func() (string, error){
		func() (string, error) {
			return "get", testClient.get(context.Background(), testPath, nil)
		},
		func() (string, error) {
			return "put", testClient.put(context.Background(), testPath, nil, nil)
		},
		func() (string, error) {
			return "delete", testClient.delete(context.Background(), testPath)
		},
	}

	for _, f := range requests {
		name, err := f()
		assert.NoError(t, err, fmt.Sprintf("%s should not return an error", name))
	}
}

func TestAPIError_Error(t *testing.T) {
	type fields struct {
		apiError error
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "Elasticsearch JSON error response",
			fields: fields{newAPIError(context.Background(), &http.Response{
				StatusCode: 400,
				Status:     "400 Bad Request",
				Body:       io.NopCloser(bytes.NewBufferString(fixtures.ErrorSample)),
			})},
			want: `400 Bad Request: {Status:400 Error:{CausedBy:{Reason:cannot set discovery.zen.minimum_master_nodes to more than the current master nodes count [1] ` +
				`Type:illegal_argument_exception} Reason:illegal value can't update [discovery.zen.minimum_master_nodes] from [1] to [6] Type:illegal_argument_exception ` +
				`StackTrace: RootCause:[{Reason:[stack-sample-es-575vhzs8ln][10.60.1.22:9300][cluster:admin/settings/update] Type:remote_transport_exception}]}}`,
		},
		{
			name: "JSON non-error response",
			fields: fields{newAPIError(context.Background(), &http.Response{
				StatusCode: 408,
				Status:     "408 Request Timeout",
				Body:       io.NopCloser(bytes.NewBufferString(fixtures.TimeoutSample)),
			})},
			want: "408 Request Timeout: {Status:0 Error:{CausedBy:{Reason: Type:} Reason: Type: StackTrace: RootCause:[]}}",
		},

		{
			name: "non-JSON error response",
			fields: fields{newAPIError(context.Background(), &http.Response{
				StatusCode: 500,
				Status:     "500 Internal Server Error",
				Body:       io.NopCloser(bytes.NewBufferString("")),
			})},
			want: "500 Internal Server Error: {Status:0 Error:{CausedBy:{Reason: Type:} Reason: Type: StackTrace: RootCause:[]}}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fields.apiError.Error(); got != tt.want {
				t.Errorf("APIError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClientGetNodes(t *testing.T) {
	expectedPath := "/_nodes/_all/no-metrics"
	testClient := NewMockClient(version.MustParse("6.8.0"), func(req *http.Request) *http.Response {
		require.Equal(t, expectedPath, req.URL.Path)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(fixtures.NodesSample)),
			Header:     make(http.Header),
			Request:    req,
		}
	})
	resp, err := testClient.GetNodes(context.Background())
	require.NoError(t, err)
	require.Equal(t, 3, len(resp.Nodes))
	require.Contains(t, resp.Nodes, "iXqjbgPYThO-6S7reL5_HA")
	require.ElementsMatch(t, []string{"master", "data", "ingest"}, resp.Nodes["iXqjbgPYThO-6S7reL5_HA"].Roles)
}

func TestClientGetNodesStats(t *testing.T) {
	expectedPath := "/_nodes/_all/stats/os"
	testClient := NewMockClient(version.MustParse("6.8.0"), func(req *http.Request) *http.Response {
		require.Equal(t, expectedPath, req.URL.Path)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(fixtures.NodesStatsSample)),
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
			Body:       io.NopCloser(strings.NewReader(fixtures.InfoSample)),
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
	dummyEndpoint := NewStaticURLProvider("es-url")
	dummyUser := BasicAuth{Name: "user", Password: "password"}
	dummyNamespaceName := types.NamespacedName{
		Namespace: "ns",
		Name:      "es",
	}
	createCert := func() *x509.Certificate {
		ca, err := certificates.NewSelfSignedCA(certificates.CABuilderOptions{})
		require.NoError(t, err)
		return ca.Cert
	}
	dummyCACerts := []*x509.Certificate{createCert()}
	v6 := version.MustParse("6.8.0")
	v7 := version.MustParse("7.0.0")
	timeout := Timeout(context.Background(), esv1.Elasticsearch{})
	x509.NewCertPool()
	tests := []struct {
		name string
		c1   Client
		c2   Client
		want bool
	}{
		{
			name: "c1 and c2 equals",
			c1:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, v6, dummyCACerts, timeout, false),
			c2:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, v6, dummyCACerts, timeout, false),
			want: true,
		},
		{
			name: "c2 nil",
			c1:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, v6, dummyCACerts, timeout, false),
			c2:   nil,
			want: false,
		},
		{
			name: "different endpoint",
			c1:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, v6, dummyCACerts, timeout, false),
			c2:   NewElasticsearchClient(nil, dummyNamespaceName, NewStaticURLProvider("another-endpoint"), dummyUser, v6, dummyCACerts, timeout, false),
			want: false,
		},
		{
			name: "different user",
			c1:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, v6, dummyCACerts, timeout, false),
			c2:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, BasicAuth{Name: "user", Password: "another-password"}, v6, dummyCACerts, timeout, false),
			want: false,
		},
		{
			name: "different CA cert",
			c1:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, v6, dummyCACerts, timeout, false),
			c2:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, v6, []*x509.Certificate{createCert()}, timeout, false),
			want: false,
		},
		{
			name: "different CA certs length",
			c1:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, v6, dummyCACerts, timeout, false),
			c2:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, v6, []*x509.Certificate{createCert(), createCert()}, timeout, false),
			want: false,
		},
		{
			name: "different dialers are not taken into consideration",
			c1:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, v6, dummyCACerts, timeout, false),
			c2:   NewElasticsearchClient(portforward.NewForwardingDialer(), dummyNamespaceName, dummyEndpoint, dummyUser, v6, dummyCACerts, timeout, false),
			want: true,
		},
		{
			name: "different versions",
			c1:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, v6, dummyCACerts, timeout, false),
			c2:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, v7, dummyCACerts, timeout, false),
			want: false,
		},
		{
			name: "same versions",
			c1:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, v7, dummyCACerts, timeout, false),
			c2:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, v7, dummyCACerts, timeout, false),
			want: true,
		},
		{
			name: "one has a version",
			c1:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, v7, dummyCACerts, timeout, false),
			c2:   NewElasticsearchClient(nil, dummyNamespaceName, dummyEndpoint, dummyUser, version.Version{}, dummyCACerts, timeout, false),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.True(t, tt.c1.Equal(tt.c2) == tt.want)
		})
	}
}

func TestClient_AddVotingConfigExclusions(t *testing.T) {
	tests := []struct {
		expectedPath  string
		expectedQuery string
		version       version.Version
		wantErr       bool
	}{
		{
			expectedPath: "",
			version:      version.MustParse("6.8.0"),
			wantErr:      true,
		},
		{
			expectedPath: "/_cluster/voting_config_exclusions/a,b",
			version:      version.MustParse("7.0.0"),
			wantErr:      false,
		},
		{
			expectedPath:  "/_cluster/voting_config_exclusions",
			expectedQuery: "node_names=a,b",
			version:       version.MustParse("7.8.0"),
			wantErr:       false,
		},
		{
			expectedPath:  "/_cluster/voting_config_exclusions",
			expectedQuery: "node_names=a,b",
			version:       version.MustParse("8.0.0"),
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		client := NewMockClient(tt.version, func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path, tt.version)
			require.Equal(t, tt.expectedQuery, req.URL.RawQuery, tt.version)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("")),
			}
		})
		err := client.AddVotingConfigExclusions(context.Background(), []string{"a", "b"})
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
			version:      version.MustParse("6.8.0"),
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
				Body:       io.NopCloser(strings.NewReader("")),
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
			version:      version.MustParse("6.8.0"),
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
				Body:       io.NopCloser(strings.NewReader("")),
			}
		})
		err := client.SetMinimumMasterNodes(context.Background(), 1)
		if (err != nil) != tt.wantErr {
			t.Errorf("Client.SetMinimumMasterNodes() error = %v, wantErr %v", err, tt.wantErr)
		}
	}
}

func TestAPIError_Types(t *testing.T) {
	ctx := context.Background()
	type args struct {
		err error
	}
	tests := []struct {
		name          string
		args          args
		wantConflict  bool
		wantForbidden bool
		wantNotFound  bool
	}{
		{
			name: "500 is not any of the explicitly supported error types",
			args: args{
				err: newAPIError(ctx, NewMockResponse(500, nil, "")), //nolint:bodyclose
			},
		},
		{
			name: "409 is a conflict",
			args: args{
				err: newAPIError(ctx, NewMockResponse(409, nil, "")), //nolint:bodyclose
			},
			wantConflict: true,
		},
		{
			name: "403 is a forbidden",
			args: args{
				err: newAPIError(ctx, NewMockResponse(403, nil, "")), //nolint:bodyclose
			},
			wantForbidden: true,
		},
		{
			name: "404 is not found",
			args: args{
				err: newAPIError(ctx, NewMockResponse(404, nil, "")), //nolint:bodyclose
			},
			wantNotFound: true,
		},
		{
			name: "no api error",
			args: args{
				err: errors.New("not an api error"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.args.err); got != tt.wantNotFound {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.wantNotFound)
			}

			if got := IsForbidden(tt.args.err); got != tt.wantForbidden {
				t.Errorf("IsForbidden() = %v, want %v", got, tt.wantForbidden)
			}
			if got := IsConflict(tt.args.err); got != tt.wantConflict {
				t.Errorf("IsConflict() = %v, want %v", got, tt.wantConflict)
			}
		})
	}
}

func TestClient_ClusterBootstrappedForZen2(t *testing.T) {
	tests := []struct {
		name                               string
		expectedPath, version, apiResponse string
		bootstrappedForZen2, wantErr       bool
	}{
		{
			name:                "6.x master node",
			expectedPath:        "/_nodes/_master",
			version:             "6.8.0",
			apiResponse:         fixtures.MasterNodeForVersion("6.8.0"),
			bootstrappedForZen2: false,
			wantErr:             false,
		},
		{
			name:                "7.x master node",
			expectedPath:        "/_nodes/_master",
			version:             "7.5.0",
			apiResponse:         fixtures.MasterNodeForVersion("7.5.0"),
			bootstrappedForZen2: true,
			wantErr:             false,
		},
		{
			name:                "no master node",
			expectedPath:        "/_nodes/_master",
			version:             "7.5.0",
			apiResponse:         `{"cluster_name": "elasticsearch-sample", "nodes": {}}`,
			bootstrappedForZen2: false,
			wantErr:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMockClient(version.MustParse(tt.version), func(req *http.Request) *http.Response {
				require.Equal(t, tt.expectedPath, req.URL.Path)
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(tt.apiResponse)),
				}
			})
			bootstrappedForZen2, err := client.ClusterBootstrappedForZen2(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Client.ClusterBootstrappedForZen2() error = %v, wantErr %v", err, tt.wantErr)
			}
			require.Equal(t, tt.bootstrappedForZen2, bootstrappedForZen2)
		})
	}
}

func TestTimeout(t *testing.T) {
	es := esv1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "test", Annotations: map[string]string{ESClientTimeoutAnnotation: "1m"}}}
	have := Timeout(context.Background(), es)
	require.Equal(t, 1*time.Minute, have)
}

func TestFormatAsSeconds(t *testing.T) {
	have := formatAsSeconds(2 * time.Minute)
	require.Equal(t, "120s", have)
}

func TestGetClusterHealthWaitForAllEvents(t *testing.T) {
	expectedPath := "/_cluster/health"
	expectedRawQuery := "wait_for_events=languid&timeout=0s"
	type fields struct {
		client Client
	}
	tests := []struct {
		name    string
		fields  fields
		want    Health
		wantErr bool
	}{
		{
			name: "other than 408 should not be ignored",
			fields: fields{
				client: NewMockClient(version.MustParse("7.14.0"), func(req *http.Request) *http.Response {
					require.Equal(t, expectedPath, req.URL.Path)
					require.Equal(t, expectedRawQuery, req.URL.RawQuery)
					return &http.Response{
						StatusCode: 500,
					}
				}),
			},
			wantErr: true,
		},
		{
			name: "408 should be ignored",
			fields: fields{
				client: NewMockClient(version.MustParse("7.14.0"), func(req *http.Request) *http.Response {
					require.Equal(t, expectedPath, req.URL.Path)
					require.Equal(t, expectedRawQuery, req.URL.RawQuery)
					return &http.Response{
						StatusCode: 408,
						Body: io.NopCloser(strings.NewReader(
							`
{
	"cluster_name": "elasticsearch-sample",
	"status": "green",
	"timed_out": true,
	"number_of_nodes": 3,
	"number_of_data_nodes": 3,
	"active_primary_shards": 12,
	"active_shards": 24,
	"relocating_shards": 1,
	"initializing_shards": 2,
	"unassigned_shards": 3,
	"delayed_unassigned_shards": 4,
	"number_of_pending_tasks": 5,
	"number_of_in_flight_fetch": 6,
	"task_max_waiting_in_queue_millis": 7,
	"active_shards_percent_as_number": 42.0
}`,
						)),
						Header:  make(http.Header),
						Request: req,
					}
				}),
			},
			want: Health{
				ClusterName:                 "elasticsearch-sample",
				Status:                      "green",
				TimedOut:                    true,
				NumberOfNodes:               3,
				NumberOfDataNodes:           3,
				ActivePrimaryShards:         12,
				ActiveShards:                24,
				RelocatingShards:            1,
				InitializingShards:          2,
				UnassignedShards:            3,
				DelayedUnassignedShards:     4,
				NumberOfPendingTasks:        5,
				NumberOfInFlightFetch:       6,
				TaskMaxWaitingInQueueMillis: 7,
				ActiveShardsPercentAsNumber: 42.0,
			},
		},
		{
			name: "happy path",
			fields: fields{
				client: NewMockClient(version.MustParse("7.14.0"), func(req *http.Request) *http.Response {
					require.Equal(t, expectedPath, req.URL.Path)
					require.Equal(t, expectedRawQuery, req.URL.RawQuery)
					return &http.Response{
						StatusCode: 200,
						Body: io.NopCloser(strings.NewReader(
							`
{
	"cluster_name": "elasticsearch-sample",
	"status": "yellow",
	"timed_out": false,
	"number_of_nodes": 3,
	"number_of_data_nodes": 3,
	"active_primary_shards": 12,
	"active_shards": 24,
	"relocating_shards": 1,
	"initializing_shards": 2,
	"unassigned_shards": 3,
	"delayed_unassigned_shards": 4,
	"number_of_pending_tasks": 5,
	"number_of_in_flight_fetch": 6,
	"task_max_waiting_in_queue_millis": 7,
	"active_shards_percent_as_number": 42.0
}`,
						)),
						Header:  make(http.Header),
						Request: req,
					}
				}),
			},
			want: Health{
				ClusterName:                 "elasticsearch-sample",
				Status:                      "yellow",
				TimedOut:                    false,
				NumberOfNodes:               3,
				NumberOfDataNodes:           3,
				ActivePrimaryShards:         12,
				ActiveShards:                24,
				RelocatingShards:            1,
				InitializingShards:          2,
				UnassignedShards:            3,
				DelayedUnassignedShards:     4,
				NumberOfPendingTasks:        5,
				NumberOfInFlightFetch:       6,
				TaskMaxWaitingInQueueMillis: 7,
				ActiveShardsPercentAsNumber: 42.0,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fields.client.GetClusterHealthWaitForAllEvents(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("GetClusterHealthWaitForAllEvents() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetClusterHealthWaitForAllEvents() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_HasProperties(t *testing.T) {
	defaultVersion := version.MustParse("8.6.1")
	defaultUser := BasicAuth{Name: "foo", Password: "bar"}
	defaultURLProvider := NewStaticURLProvider("https://foo.bar")
	defaultCaCerts := []*x509.Certificate{{Raw: []byte("foo")}}
	defaultEsClient := NewElasticsearchClient(
		nil,
		types.NamespacedName{Namespace: "ns", Name: "es"},
		defaultURLProvider,
		defaultUser,
		defaultVersion,
		defaultCaCerts,
		Timeout(context.Background(), esv1.Elasticsearch{}),
		false,
	)
	tests := []struct {
		name     string
		esClient Client
		version  version.Version
		user     BasicAuth
		url      URLProvider
		caCerts  []*x509.Certificate
		want     bool
	}{
		{
			name:     "A new client is created if the version does not match",
			esClient: defaultEsClient,
			version:  version.MustParse("8.6.0"),
			user:     defaultUser,
			url:      defaultURLProvider,
			caCerts:  defaultCaCerts,
			want:     false,
		},
		{
			name:     "A new client is created if the user does not match",
			esClient: defaultEsClient,
			version:  defaultVersion,
			user:     BasicAuth{Name: "foo", Password: "changed"},
			url:      defaultURLProvider,
			caCerts:  defaultCaCerts,
			want:     false,
		},
		{
			name:     "A new client is created if the url does not match",
			esClient: defaultEsClient,
			version:  defaultVersion,
			user:     defaultUser,
			url:      NewStaticURLProvider("https://foo.com"),
			caCerts:  defaultCaCerts,
			want:     false,
		},
		{
			name:     "A new client is created if the caCerts do not match",
			esClient: defaultEsClient,
			version:  defaultVersion,
			user:     defaultUser,
			url:      defaultURLProvider,
			caCerts:  []*x509.Certificate{{Raw: []byte("bar")}},
			want:     false,
		},
		{
			name:     "The client is reused if nothing has changed",
			esClient: defaultEsClient,
			version:  defaultVersion,
			user:     defaultUser,
			url:      defaultURLProvider,
			caCerts:  defaultCaCerts,
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.esClient.HasProperties(tt.version, tt.user, tt.url, tt.caCerts)
			assert.Equal(t, tt.want, actual)
		})
	}
}
