// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
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
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				"stack-sample-es-lkrjf7224s": {{Index: "sample-data-2", Shard: "0", State: STARTED, NodeName: "stack-sample-es-lkrjf7224s"}},
				"stack-sample-es-4fxm76vnwj": {{Index: "sample-data-2", Shard: "1", State: STARTED, NodeName: "stack-sample-es-4fxm76vnwj"}},
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
			Body:       ioutil.NopCloser(&bytes.Buffer{}),
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
	testClient, err := NewMockClient(version.MustParse("6.8.0"), errorResponses(codes))
	require.NoError(t, err)

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

func TestClient_Request(t *testing.T) {
	testPath := "/_i_am_an/elasticsearch/endpoint"
	testClient, err := NewMockClientWithUser(
		version.MustParse("6.8.0"),
		UserAuth{Name: "foo", Password: "bar"},
		requestAssertion(func(req *http.Request) {
			assert.Equal(t, testPath, req.URL.Path)
		}),
	)
	require.NoError(t, err)

	requests := []func() (string, error){
		func() (string, error) {
			req, err := http.NewRequest(http.MethodGet, testPath, nil)
			require.NoError(t, err)
			r, err := testClient.Request(context.Background(), req)
			if r != nil {
				require.NoError(t, r.Body.Close())
			}
			return "get", err
		},
		func() (string, error) {
			req, err := http.NewRequest(http.MethodPut, testPath, nil)
			require.NoError(t, err)
			r, err := testClient.Request(context.Background(), req)
			if r != nil {
				require.NoError(t, r.Body.Close())
			}
			return "put", err
		},
		func() (string, error) {
			req, err := http.NewRequest(http.MethodDelete, testPath, nil)
			require.NoError(t, err)
			r, err := testClient.Request(context.Background(), req)
			if r != nil {
				require.NoError(t, r.Body.Close())
			}
			return "delete", err
		},
	}

	for _, f := range requests {
		name, err := f()
		assert.NoError(t, err, fmt.Sprintf("%s should not return an error", name))
	}
}

func TestOfficialAPIError_Error(t *testing.T) {
	type fields struct {
		response *Response
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "Elasticsearch JSON error response",
			fields: fields{&Response{
				StatusCode: http.StatusBadRequest,
				Body:       ioutil.NopCloser(bytes.NewBufferString(fixtures.ErrorSample)),
			}},
			want: "400: illegal value can't update [discovery.zen.minimum_master_nodes] from [1] to [6]",
		},
		{
			name: "non-JSON error response",
			fields: fields{&Response{
				StatusCode: http.StatusInternalServerError,
				Body:       ioutil.NopCloser(bytes.NewBufferString("")),
			}},
			want: "500: unknown (decode error: EOF)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newOfficialAPIError(tt.fields.response)

			if got := e.Error(); got != tt.want {
				t.Errorf("officialAPIError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClientGetNodes(t *testing.T) {
	expectedPath := "/_nodes/_all/jvm,settings"
	testClient, err := NewMockClient(version.MustParse("6.8.0"), func(req *http.Request) *http.Response {
		require.Equal(t, expectedPath, req.URL.Path)
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(fixtures.NodesSample)),
			Header:     make(http.Header),
			Request:    req,
		}
	})
	require.NoError(t, err)

	resp, err := testClient.GetNodes(context.Background())
	require.NoError(t, err)
	require.Equal(t, 3, len(resp.Nodes))
	require.Contains(t, resp.Nodes, "iXqjbgPYThO-6S7reL5_HA")
	require.ElementsMatch(t, []string{"master", "data", "ingest"}, resp.Nodes["iXqjbgPYThO-6S7reL5_HA"].Roles)
	require.Equal(t, 2130051072, resp.Nodes["iXqjbgPYThO-6S7reL5_HA"].JVM.Mem.HeapMaxInBytes)
}

func TestClientGetNodesStats(t *testing.T) {
	expectedPath := "/_nodes/_all/stats/os"
	testClient, err := NewMockClient(version.MustParse("6.8.0"), func(req *http.Request) *http.Response {
		require.Equal(t, expectedPath, req.URL.Path)
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(fixtures.NodesStatsSample)),
			Header:     make(http.Header),
			Request:    req,
		}
	})
	require.NoError(t, err)

	resp, err := testClient.GetNodesStats(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, len(resp.Nodes))
	require.Contains(t, resp.Nodes, "Rt-o5-ZBQaq-Nkhhy0p7JA")
	require.Equal(t, "3221225472", resp.Nodes["Rt-o5-ZBQaq-Nkhhy0p7JA"].OS.CGroup.Memory.LimitInBytes)
}

func TestGetInfo(t *testing.T) {
	expectedPath := "/"
	testClient, err := NewMockClient(version.MustParse("6.4.1"), func(req *http.Request) *http.Response {
		require.Equal(t, expectedPath, req.URL.Path)
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(strings.NewReader(fixtures.InfoSample)),
			Header:     make(http.Header),
			Request:    req,
		}
	})
	require.NoError(t, err)
	info, err := testClient.GetClusterInfo(context.Background())
	require.NoError(t, err)
	require.Equal(t, "af932d24216a4dd69ba47d2fd3214796", info.ClusterName)
	require.Equal(t, "LGA3VblKTNmzP6Q6SWxfkw", info.ClusterUUID)
	require.Equal(t, "6.4.1", info.Version.Number)
}

func newElasticsearchClient(
	t *testing.T,
	dialer net.Dialer,
	esURL string,
	esUser UserAuth,
	v version.Version,
	caCerts []*x509.Certificate,
) Client {
	c, err := NewDefaultElasticsearchClient(dialer, esURL, esUser, v, caCerts)
	require.NoError(t, err)
	return c
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
	v6 := version.MustParse("6.8.0")
	v7 := version.MustParse("7.0.0")
	x509.NewCertPool()
	tests := []struct {
		name string
		c1   func(t *testing.T) Client
		c2   func(t *testing.T) Client
		want bool
	}{
		{
			name: "c1 and c2 equals",
			c1: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, v6, dummyCACerts)
			},
			c2: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, v6, dummyCACerts)
			},
			want: true,
		},
		{
			name: "c2 nil",
			c1: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, v6, dummyCACerts)
			},
			c2: func(t *testing.T) Client {
				return nil
			},
			want: false,
		},
		{
			name: "different endpoint",
			c1: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, v6, dummyCACerts)
			},
			c2: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, "another-endpoint", dummyUser, v6, dummyCACerts)
			},
			want: false,
		},
		{
			name: "different user",
			c1: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, v6, dummyCACerts)
			},
			c2: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, UserAuth{Name: "user", Password: "another-password"}, v6, dummyCACerts)
			},
			want: false,
		},
		{
			name: "different CA cert",
			c1: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, v6, dummyCACerts)
			},
			c2: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, v6, []*x509.Certificate{createCert()})
			},
			want: false,
		},
		{
			name: "different CA certs length",
			c1: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, v6, dummyCACerts)
			},
			c2: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, v6, []*x509.Certificate{createCert(), createCert()})
			},
			want: false,
		},
		{
			name: "different dialers are not taken into consideration",
			c1: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, v6, dummyCACerts)
			},
			c2: func(t *testing.T) Client {
				return newElasticsearchClient(t, portforward.NewForwardingDialer(), dummyEndpoint, dummyUser, v6, dummyCACerts)
			},
			want: true,
		},
		{
			name: "different versions",
			c1: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, v6, dummyCACerts)
			},
			c2: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, v7, dummyCACerts)
			},
			want: false,
		},
		{
			name: "same versions",
			c1: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, v7, dummyCACerts)
			},
			c2: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, v7, dummyCACerts)
			},
			want: true,
		},
		{
			name: "one has a version",
			c1: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, v7, dummyCACerts)
			},
			c2: func(t *testing.T) Client {
				return newElasticsearchClient(t, nil, dummyEndpoint, dummyUser, version.Version{}, dummyCACerts)
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c1 := tt.c1(t)
			c2 := tt.c2(t)
			require.True(t, c1.Equal(c2) == tt.want)
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
			version:      version.MustParse("6.8.0"),
		},
		{
			expectedPath: "/_license",
			version:      version.MustParse("7.0.0"),
		},
	}
	for _, tt := range tests {
		testClient, err := NewMockClient(tt.version, func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader(fixtures.LicenseUpdateResponseSample)),
				Header:     make(http.Header),
				Request:    req,
			}
		})
		require.NoError(t, err)

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
			version:      version.MustParse("6.8.0"),
		},
		{
			expectedPath: "/_license",
			version:      version.MustParse("7.0.0"),
		},
	}

	for _, tt := range tests {
		testClient, err := NewMockClient(tt.version, func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader(fixtures.LicenseGetSample)),
				Header:     make(http.Header),
				Request:    req,
			}
		})
		require.NoError(t, err)

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
			version:      version.MustParse("6.8.0"),
			wantErr:      true,
		},
		{
			expectedPath: "/_cluster/voting_config_exclusions/a,b",
			version:      version.MustParse("7.0.0"),
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		client, err := NewMockClient(tt.version, func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader("")),
			}
		})
		require.NoError(t, err)

		err = client.AddVotingConfigExclusions(context.Background(), []string{"a", "b"})
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
		client, err := NewMockClient(tt.version, func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader("")),
			}
		})
		require.NoError(t, err)

		err = client.DeleteVotingConfigExclusions(context.Background())
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
		client, err := NewMockClient(tt.version, func(req *http.Request) *http.Response {
			require.Equal(t, tt.expectedPath, req.URL.Path)
			return &http.Response{
				StatusCode: 200,
				Body:       ioutil.NopCloser(strings.NewReader("")),
			}
		})
		require.NoError(t, err)

		err = client.SetMinimumMasterNodes(context.Background(), 1)
		if (err != nil) != tt.wantErr {
			t.Errorf("Client.SetMinimumMasterNodes() error = %v, wantErr %v", err, tt.wantErr)
		}
	}
}

func TestIsConflict(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "200 is not a conflict",
			args: args{
				err: &officialAPIError{
					statusCode:    http.StatusOK,
					errorResponse: nil,
				}, // nolint
			},
			want: false,
		},
		{
			name: "409 is a conflict",
			args: args{
				err: &officialAPIError{
					statusCode:    http.StatusConflict,
					errorResponse: nil,
				}, // nolint
			},
			want: true,
		},
		{
			name: "no api error",
			args: args{
				err: errors.New("not an api error"),
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsConflict(tt.args.err); got != tt.want {
				t.Errorf("IsConflict() = %v, want %v", got, tt.want)
			}
		})
	}
}
