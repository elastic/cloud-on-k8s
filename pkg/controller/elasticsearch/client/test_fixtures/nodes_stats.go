// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package fixtures

const (
	NodesStatsSample = `
{
  "_nodes" : {
    "total" : 1,
    "successful" : 1,
    "failed" : 0
  },
  "cluster_name" : "elasticsearch-sample",
  "nodes" : {
    "Rt-o5-ZBQaq-Nkhhy0p7JA" : {
      "timestamp" : 1560016895151,
      "name" : "elasticsearch-sample-es-jq5xknvpkf",
      "transport_address" : "10.68.0.200:9300",
      "host" : "10.68.0.200",
      "ip" : "10.68.0.200:9300",
      "roles" : [
        "master",
        "data",
        "ingest"
      ],
      "attributes" : {
        "ml.machine_memory" : "3221225472",
        "xpack.installed" : "true",
        "ml.max_open_jobs" : "20"
      },
      "os" : {
        "timestamp" : 1560016895152,
        "cpu" : {
          "percent" : 19,
          "load_average" : {
            "1m" : 0.69,
            "5m" : 0.67,
            "15m" : 0.5
          }
        },
        "mem" : {
          "total_in_bytes" : 27395481600,
          "free_in_bytes" : 1319841792,
          "used_in_bytes" : 26075639808,
          "free_percent" : 5,
          "used_percent" : 95
        },
        "swap" : {
          "total_in_bytes" : 0,
          "free_in_bytes" : 0,
          "used_in_bytes" : 0
        },
        "cgroup" : {
          "cpuacct" : {
            "control_group" : "/",
            "usage_nanos" : 124238368912
          },
          "cpu" : {
            "control_group" : "/",
            "cfs_period_micros" : 100000,
            "cfs_quota_micros" : -1,
            "stat" : {
              "number_of_elapsed_periods" : 0,
              "number_of_times_throttled" : 0,
              "time_throttled_nanos" : 0
            }
          },
          "memory" : {
            "control_group" : "/",
            "limit_in_bytes" : "3221225472",
            "usage_in_bytes" : "2926161920"
          }
        }
      }
    }
  }
}`
)
