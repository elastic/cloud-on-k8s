// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package fixtures

const (
	ClusterStateSample = `
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
	EmptyClusterStateSample = `
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
