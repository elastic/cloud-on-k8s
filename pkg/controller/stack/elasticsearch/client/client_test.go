package client

import (
	"encoding/json"
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
              "primary": true,
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
  },
  "routing_nodes": {
    "unassigned": [
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
    ],
    "nodes": {
      "SaGT6YMJQyS409ZhonOLhQ": [
        {
          "state": "STARTED",
          "primary": true,
          "node": "SaGT6YMJQyS409ZhonOLhQ",
          "relocating_node": null,
          "shard": 1,
          "index": "sample-data-2",
          "allocation_id": {
            "id": "llMZRy1jTA-Fe_X1jDBvnw"
          }
        }
      ],
      "4cHWfQAwQQKTvKV1vrtbDQ": [
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
      ]
    }
  },
  "snapshot_deletions": {
    "snapshot_deletions": []
  },
  "snapshots": {
    "snapshots": []
  },
  "restore": {
    "snapshots": []
  }
}
`
)

func TestParseRoutingTable(t *testing.T) {

	expected := []Shard{
		Shard{Index: "sample-data-2", Shard: 0, Primary: true, State: STARTED, Node: "stack-sample-es-lkrjf7224s"},
		Shard{Index: "sample-data-2", Shard: 1, Primary: true, State: STARTED, Node: "stack-sample-es-4fxm76vnwj"},
		Shard{Index: "sample-data-2", Shard: 2, Primary: true, State: UNASSIGNED, Node: ""},
	}
	var unstructured interface{}
	b := []byte(ClusterDataSample)
	err := json.Unmarshal(b, &unstructured)
	if err != nil {
		t.Error(err)
	}
	shards, e := parseRoutingTable(unstructured)
	t.Log(shards)
	assert.NoError(t, e, "should parse without error")
	assert.True(t, len(shards) == 3)
	sort.SliceStable(shards, func(i, j int) bool {
		return shards[i].Shard < shards[j].Shard
	})
	for i := range shards {
		assert.EqualValues(t, expected[i], shards[i])
	}

}
