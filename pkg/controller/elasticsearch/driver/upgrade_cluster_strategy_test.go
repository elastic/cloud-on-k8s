package driver

const (
	sampleClusterState = `{
  "cluster_name" : "elasticsearch-sample",
  "cluster_uuid" : "n3tTyyoyTlqsZ0xMwekFWw",
  "master_node" : "GA-ZgR0bRC64iPAoXRAwng",
  "nodes" : {
    "J_D32LThRSW8EOsDQ8al0A" : {
      "name" : "elasticsearch-sample-es-nodes-1",
      "ephemeral_id" : "dUXjWNtBSDOYEl7vEAv3hw",
      "transport_address" : "10.233.66.157:9300",
      "attributes" : {
        "ml.machine_memory" : "4294967296",
        "ml.max_open_jobs" : "20",
        "xpack.installed" : "true",
        "attr_name" : "attr_value"
      }
    },
    "n9MS8Bn8R1m-T9u76wU45w" : {
      "name" : "elasticsearch-sample-es-masters-0",
      "ephemeral_id" : "_MoCcpNMRDy2PCbDjV4P6w",
      "transport_address" : "10.233.65.34:9300",
      "attributes" : {
        "attr_name" : "attr_value",
        "xpack.installed" : "true"
      }
    },
    "lwr67QTrRdqTGfD_bN2tPQ" : {
      "name" : "elasticsearch-sample-es-masters-1",
      "ephemeral_id" : "767pD01ZQAyS9PcTg6YixA",
      "transport_address" : "10.233.66.158:9300",
      "attributes" : {
        "attr_name" : "attr_value",
        "xpack.installed" : "true"
      }
    },
    "CBOVABG9QNGLGh1w23UGsg" : {
      "name" : "elasticsearch-sample-es-nodes-3",
      "ephemeral_id" : "nxr1tnJCReCJGv6t3rvakA",
      "transport_address" : "10.233.65.58:9300",
      "attributes" : {
        "ml.machine_memory" : "4294967296",
        "ml.max_open_jobs" : "20",
        "xpack.installed" : "true",
        "attr_name" : "attr_value"
      }
    },
    "ZRz1d_mLQq-GbY-ceYaGuQ" : {
      "name" : "elasticsearch-sample-es-nodes-0",
      "ephemeral_id" : "tp9l5jWXTZC_r3k1PbjtUg",
      "transport_address" : "10.233.65.46:9300",
      "attributes" : {
        "ml.machine_memory" : "4294967296",
        "ml.max_open_jobs" : "20",
        "xpack.installed" : "true",
        "attr_name" : "attr_value"
      }
    },
    "GA-ZgR0bRC64iPAoXRAwng" : {
      "name" : "elasticsearch-sample-es-masters-2",
      "ephemeral_id" : "6kV_nWmPSFO_whRL2IPV4Q",
      "transport_address" : "10.233.67.124:9300",
      "attributes" : {
        "attr_name" : "attr_value",
        "xpack.installed" : "true"
      }
    },
    "DeyNLGZBSM6jLaExq4glzQ" : {
      "name" : "elasticsearch-sample-es-nodes-2",
      "ephemeral_id" : "__kzR3lCTwamotedejJMmA",
      "transport_address" : "10.233.67.15:9300",
      "attributes" : {
        "ml.machine_memory" : "4294967296",
        "ml.max_open_jobs" : "20",
        "xpack.installed" : "true",
        "attr_name" : "attr_value"
      }
    },
    "lgiq13sPRVWlBiPTLaTivA" : {
      "name" : "elasticsearch-sample-es-nodes-4",
      "ephemeral_id" : "YrXGoHfkS9KQLYhZbAxcQw",
      "transport_address" : "10.233.66.142:9300",
      "attributes" : {
        "ml.machine_memory" : "4294967296",
        "ml.max_open_jobs" : "20",
        "xpack.installed" : "true",
        "attr_name" : "attr_value"
      }
    }
  },
  "routing_table" : {
    "indices" : {
      ".security-7" : {
        "shards" : {
          "0" : [
            {
              "state" : "STARTED",
              "primary" : false,
              "node" : "ZRz1d_mLQq-GbY-ceYaGuQ",
              "relocating_node" : null,
              "shard" : 0,
              "index" : ".security-7",
              "allocation_id" : {
                "id" : "7mnstP8WSUahLLTHuEvVoA"
              }
            },
            {
              "state" : "STARTED",
              "primary" : true,
              "node" : "J_D32LThRSW8EOsDQ8al0A",
              "relocating_node" : null,
              "shard" : 0,
              "index" : ".security-7",
              "allocation_id" : {
                "id" : "xKxkOUGsRMe0XiLDhq-w3g"
              }
            }
          ]
        }
      },
      "twitter" : {
        "shards" : {
          "0" : [
            {
              "state" : "STARTED",
              "primary" : true,
              "node" : "DeyNLGZBSM6jLaExq4glzQ",
              "relocating_node" : null,
              "shard" : 0,
              "index" : "twitter",
              "allocation_id" : {
                "id" : "5YJuiwmVTMu9r13o4s_Ziw"
              }
            }
          ]
        }
      },
      ".kibana_1" : {
        "shards" : {
          "0" : [
            {
              "state" : "STARTED",
              "primary" : true,
              "node" : "CBOVABG9QNGLGh1w23UGsg",
              "relocating_node" : null,
              "shard" : 0,
              "index" : ".kibana_1",
              "allocation_id" : {
                "id" : "VFC99m2RSXmPbUS6U_nR0Q"
              }
            },
            {
              "state" : "STARTED",
              "primary" : false,
              "node" : "lgiq13sPRVWlBiPTLaTivA",
              "relocating_node" : null,
              "shard" : 0,
              "index" : ".kibana_1",
              "allocation_id" : {
                "id" : "6fAfn-uITu6yYQ6zsGB97A"
              }
            }
          ]
        }
      }
    }
  }
}
`
)
