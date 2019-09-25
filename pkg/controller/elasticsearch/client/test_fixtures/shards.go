// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package fixtures

const (
	SampleShards = `
[{
		"state": "STARTED",
		"prirep": "p",
		"node": "stack-sample-es-lkrjf7224s",
		"shard": "0",
		"index": "sample-data-2"
	},
	{
		"state": "STARTED",
		"prirep": "r",
		"node": "stack-sample-es-4fxm76vnwj",
		"shard": "1",
		"index": "sample-data-2"
	},
	{
		"state": "UNASSIGNED",
		"prirep": "p",
		"node": null,
		"shard": "2",
		"index": "sample-data-2"
	}
]
`
	NoShards = `
[]
`
)
