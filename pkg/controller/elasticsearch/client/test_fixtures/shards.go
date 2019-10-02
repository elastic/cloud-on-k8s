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
	RelocatingShards = `[{
"index": "data-integrity-check",
"shard": "1",
"state": "RELOCATING",
"node": "test-mutation-less-nodes-sqn9-es-masterdata-1 -> 10.56.2.33 8DqGuLtrSNyMfE2EfKNDgg test-mutation-less-nodes-sqn9-es-masterdata-0"
}, {
"index": "data-integrity-check",
"shard": "2",
"state": "RELOCATING",
"node": "test-mutation-less-nodes-sqn9-es-masterdata-2 -> 10.56.2.33 8DqGuLtrSNyMfE2EfKNDgg test-mutation-less-nodes-sqn9-es-masterdata-0"
}, {
"index": "data-integrity-check",
"shard": "0",
"state": "STARTED",
"node": "test-mutation-less-nodes-sqn9-es-masterdata-0"
}, {
"index": "data-integrity-check",
"shard": "3",
"state": "UNASSIGNED",
"node": ""
}]`
	NoShards = `
[]
`
)
