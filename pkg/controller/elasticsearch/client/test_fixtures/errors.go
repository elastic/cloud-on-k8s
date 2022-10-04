// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package fixtures

const (
	ErrorSample = `
{
    "status": 400,
    "error": {
        "caused_by": {
            "reason": "cannot set discovery.zen.minimum_master_nodes to more than the current master nodes count [1]",
            "type": "illegal_argument_exception"
        },
        "reason": "illegal value can't update [discovery.zen.minimum_master_nodes] from [1] to [6]",
        "type": "illegal_argument_exception",
        "root_cause": [
            {
                "reason": "[stack-sample-es-575vhzs8ln][10.60.1.22:9300][cluster:admin/settings/update]",
                "type": "remote_transport_exception"
            }
        ]
    }
}
`
	TimeoutSample = `
{
  "cluster_name" : "testcluster",
  "status" : "yellow",
  "timed_out" : true,
  "number_of_nodes" : 1,
  "number_of_data_nodes" : 1,
  "active_primary_shards" : 1,
  "active_shards" : 1,
  "relocating_shards" : 0,
  "initializing_shards" : 0,
  "unassigned_shards" : 1,
  "delayed_unassigned_shards": 0,
  "number_of_pending_tasks" : 1,
  "number_of_in_flight_fetch": 0,
  "task_max_waiting_in_queue_millis": 0,
  "active_shards_percent_as_number": 50.0
}
`
)
