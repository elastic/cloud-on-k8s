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

	"github.com/stretchr/testify/assert"
)

const (
	ClusterDataSample = `
{
  "cluster_name": "stack-sample",
  "compressed_size_in_bytes": 10021,
  "cluster_uuid": "LyyITZoWSlO1NYEOQ6qYsA",
  "version": 69,
  "state_uuid": "pUYeoTGiRNCXfmJB-lBSjg",
  "master_node": "4cHWfQAwQQKTvKV1vrtbDQ",
  "blocks": {},
  "nodes": {
    "SaGT6YMJQyS409ZhonOLhQ": {
      "name": "stack-sample-es-4fxm76vnwj",
      "ephemeral_id": "xUIKCkLMRt6ysOPLHwcxxg",
      "transport_address": "172.17.0.5:9300",
      "attributes": {
        "ml.machine_memory": "2147483648",
        "ml.max_open_jobs": "20",
        "xpack.installed": "true",
        "ml.enabled": "true"
      }
    },
    "4cHWfQAwQQKTvKV1vrtbDQ": {
      "name": "stack-sample-es-lkrjf7224s",
      "ephemeral_id": "dgJQM-g7RYyKO_WZbzfp8A",
      "transport_address": "172.17.0.7:9300",
      "attributes": {
        "ml.machine_memory": "2147483648",
        "ml.max_open_jobs": "20",
        "xpack.installed": "true",
        "ml.enabled": "true"
      }
    }
  },
  "routing_table": {
    "indices": {
      "sample-data-2": {
        "shards": {
          "0": [
            {
              "state": "STARTED",
              "primary": true,
              "node": "4cHWfQAwQQKTvKV1vrtbDQ",
              "relocating_node": null,
              "shard": 0,
              "index": "sample-data-2",
              "allocation_id": {
                "id": "IDGMmL6ySAWnfH8bRvNmUw"
              }
            }
          ],
          "1": [
            {
              "state": "STARTED",
              "primary": false,
              "node": "SaGT6YMJQyS409ZhonOLhQ",
              "relocating_node": null,
              "shard": 1,
              "index": "sample-data-2",
              "allocation_id": {
                "id": "llMZRy1jTA-Fe_X1jDBvnw"
              }
            }
          ],
          "2": [
            {
              "state": "UNASSIGNED",
              "primary": true,
              "node": null,
              "relocating_node": null,
              "shard": 2,
              "index": "sample-data-2",
              "recovery_source": {
                "type": "EXISTING_STORE"
              },
              "unassigned_info": {
                "reason": "NODE_LEFT",
                "at": "2018-11-04T19:52:58.923Z",
                "delayed": false,
                "details": "node_left[sTom3cUZSdaRC8zBHWhn2g]",
                "allocation_status": "no_valid_shard_copy"
              }
            }
          ]
        }
      }
    }
  }
}
`
	EmptyClusterDataSample = `
{
  "cluster_name": "stack-sample",
  "compressed_size_in_bytes": 10506,
  "cluster_uuid": "LyyITZoWSlO1NYEOQ6qYsA",
  "version": 150,
  "state_uuid": "EDJl3tuTSGeaKUossvfOfA",
  "master_node": "-M71qm0GS2-wWjPdQdyEjw",
  "blocks": {},
  "nodes": {
    "wWH74nr1TXeRNkQorC1S8A": {
      "name": "stack-sample-es-v47j276fsw",
      "ephemeral_id": "IgMivqAfTMmaqhAdKa6tow",
      "transport_address": "172.17.0.6:9300",
      "attributes": {
        "ml.machine_memory": "2147483648",
        "ml.max_open_jobs": "20",
        "xpack.installed": "true",
        "ml.enabled": "true"
      }
    },
    "-M71qm0GS2-wWjPdQdyEjw": {
      "name": "stack-sample-es-tj9s45xqz7",
      "ephemeral_id": "9S5EL-28TlisnagzU96DWA",
      "transport_address": "172.17.0.5:9300",
      "attributes": {
        "ml.machine_memory": "2147483648",
        "ml.max_open_jobs": "20",
        "xpack.installed": "true",
        "ml.enabled": "true"
      }
    },
    "Kp1mi0WEShmbJFm8aPrxiw": {
      "name": "stack-sample-es-tmbtfpscsl",
      "ephemeral_id": "WKuaCpctQtKIm7jbepGcaA",
      "transport_address": "172.17.0.3:9300",
      "attributes": {
        "ml.machine_memory": "2147483648",
        "ml.max_open_jobs": "20",
        "xpack.installed": "true",
        "ml.enabled": "true"
      }
    }
  }, 
  "routing_table": {
    "indices": {}
  }
}
`
)

func TestParseRoutingTable(t *testing.T) {

	tests := []struct {
		name string
		args string
		want []Shard
	}{
		{
			name: "Can parse populated routing table",
			args: ClusterDataSample,
			want: []Shard{
				Shard{Index: "sample-data-2", Shard: 0, Primary: true, State: STARTED, Node: "stack-sample-es-lkrjf7224s"},
				Shard{Index: "sample-data-2", Shard: 1, Primary: false, State: STARTED, Node: "stack-sample-es-4fxm76vnwj"},
				Shard{Index: "sample-data-2", Shard: 2, Primary: true, State: UNASSIGNED, Node: ""},
			},
		},
		{
			name: "Can parse an empty routing table",
			args: EmptyClusterDataSample,
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
		Endpoint: "http//does-not-matter.com",
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
			return "GeTClusterState", err
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
