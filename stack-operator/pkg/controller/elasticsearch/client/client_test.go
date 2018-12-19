package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"testing"

	fixtures "github.com/elastic/stack-operators/stack-operator/pkg/controller/elasticsearch/client/test_fixtures"
	"github.com/stretchr/testify/assert"
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
				Shard{Index: "sample-data-2", Shard: 0, Primary: true, State: STARTED, Node: "stack-sample-es-lkrjf7224s"},
				Shard{Index: "sample-data-2", Shard: 1, Primary: false, State: STARTED, Node: "stack-sample-es-4fxm76vnwj"},
				Shard{Index: "sample-data-2", Shard: 2, Primary: true, State: UNASSIGNED, Node: ""},
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

type RoundTripFunc func(req *http.Request) *http.Response

func (f RoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func NewMockClient(fn RoundTripFunc) Client {
	return Client{
		HTTP: &http.Client{
			Transport: RoundTripFunc(fn),
		},
		Endpoint: "http://example.com",
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
	testClient := NewMockClient(errorResponses(codes))
	requests := []func() (string, error){
		func() (string, error) {
			_, err := testClient.GetClusterState(context.TODO())
			return "GetClusterState", err
		},
		func() (string, error) {
			return "ExcludeFromShardAllocation", testClient.ExcludeFromShardAllocation(context.TODO(), "")
		},
		func() (string, error) {
			return "UpsertSnapshotRepository", testClient.UpsertSnapshotRepository(context.TODO(), "test", SnapshotRepository{})
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
	testClient := NewMockClient(requestAssertion(func(req *http.Request) {
		assert.Equal(t, []string{"application/json; charset=utf-8"}, req.Header["Content-Type"])
	}))
	testClient.GetClusterState(context.TODO())
	testClient.ExcludeFromShardAllocation(context.TODO(), "")
}

func TestClientSupportsBasicAuth(t *testing.T) {

	type expected struct {
		user        User
		authPresent bool
	}

	tests := []struct {
		name string
		args User
		want expected
	}{
		{
			name: "Context with user information should be respected",
			args: User{Name: "elastic", Password: "changeme"},
			want: expected{
				user:        User{Name: "elastic", Password: "changeme"},
				authPresent: true,
			},
		},
		{
			name: "Context w/o user information is ok too",
			args: User{},
			want: expected{
				user:        User{Name: "", Password: ""},
				authPresent: false,
			},
		},
	}

	for _, tt := range tests {
		testClient := NewMockClient(requestAssertion(func(req *http.Request) {
			username, password, ok := req.BasicAuth()
			assert.Equal(t, tt.want.authPresent, ok)
			assert.Equal(t, tt.want.user.Name, username)
			assert.Equal(t, tt.want.user.Password, password)
		}))
		testClient.User = tt.args
		testClient.GetClusterState(context.TODO())
		testClient.ExcludeFromShardAllocation(context.TODO(), "")
		testClient.UpsertSnapshotRepository(context.TODO(), "", SnapshotRepository{})

	}

}

func TestClient_request(t *testing.T) {
	testPath := "/_i_am_an/elasticsearch/endpoint"

	testClient := NewMockClient(requestAssertion(func(req *http.Request) {
		assert.Equal(t, testPath, req.URL.Path)
	}))
	requests := []func() (string, error){
		func() (string, error) {
			return "get", testClient.get(context.TODO(), testPath, nil)
		},
		func() (string, error) {
			return "put", testClient.put(context.TODO(), testPath, nil)
		},
		func() (string, error) {
			return "delete", testClient.delete(context.TODO(), testPath)
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
