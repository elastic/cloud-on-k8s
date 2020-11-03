// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package esconfig

import (
	"testing"

	"github.com/nsf/jsondiff"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_removeNameWrapper(t *testing.T) {
	body := `{
		"my_repository": {
		  "type": "fs",
		  "settings": {
			"location": "/tmp"
		  }
		}
	  }`

	want := `{
		"type": "fs",
		"settings": {
		  "location": "/tmp"
		}
	  }`
	url := "/_snapshot/my_repository"

	got, err := removeNameWrapper(url, []byte(body))
	require.NoError(t, err)
	opts := jsondiff.DefaultConsoleOptions()
	diff, s := jsondiff.Compare([]byte(want), []byte(got), &opts)
	assert.Equal(t, jsondiff.FullMatch, diff, "differences: %s", s)
}

func Test_removeArrayWrapper(t *testing.T) {
	body := `{
		"component_templates": [
		  {
			"name": "component_template1",
			"component_template": {
			  "template": {
				"mappings": {
				  "properties": {
					"@timestamp": {
					  "type": "date"
					}
				  }
				}
			  }
			}
		  }
		]
	  }`

	want := `{
		"template": {
		  "mappings": {
			"properties": {
			  "@timestamp": {
				"type": "date"
			  }
			}
		  }
		}
	  }`

	url := "/_component_template/component_template1"

	got, err := removeArrayWrapper(url, []byte(body))
	require.NoError(t, err)
	opts := jsondiff.DefaultConsoleOptions()
	diff, s := jsondiff.Compare([]byte(want), []byte(got), &opts)
	assert.Equal(t, jsondiff.FullMatch, diff, "differences: %s", s)
}

func Test_removeResourceWrapper(t *testing.T) {
	body := `{
		"nightly-snapshots": {
		  "version": 1,
		  "modified_date_millis": 1603409192704,
		  "policy": {
			"name": "<nightly-snap-{now/d}>",
			"schedule": "0 30 1 * * ?",
			"repository": "my_repository",
			"config": {
			  "indices": [
				"*"
			  ]
			},
			"retention": {
			  "expire_after": "30d",
			  "min_count": 5,
			  "max_count": 50
			}
		  },
		  "next_execution_millis": 1603416600000,
		  "stats": {
			"policy": "nightly-snapshots",
			"snapshots_taken": 0,
			"snapshots_failed": 0,
			"snapshots_deleted": 0,
			"snapshot_deletion_failures": 0
		  }
		}
	  }`

	want := `{
		"schedule": "0 30 1 * * ?",
		"name": "<nightly-snap-{now/d}>",
		"repository": "my_repository",
		"config": {
		  "indices": ["*"]
		},
		"retention": {
		  "expire_after": "30d",
		  "min_count": 5,
		  "max_count": 50
		}
	  }`

	url := "/_slm/policy/nightly-snapshots"

	got, err := removeResourceWrapper(url, []byte(body))
	require.NoError(t, err)
	opts := jsondiff.DefaultConsoleOptions()
	diff, s := jsondiff.Compare([]byte(want), []byte(got), &opts)
	assert.Equal(t, jsondiff.FullMatch, diff, "want: %s \n got: %s differences: %s", want, got, s)
}

func Test_ILM(t *testing.T) {
	body := `{
		"my-data-stream-policy": {
		  "version": 1,
		  "modified_date": "2020-10-22T23:08:18.710Z",
		  "policy": {
			"phases": {
			  "hot": {
				"min_age": "0ms",
				"actions": {
				  "rollover": {
					"max_size": "25GB"
				  }
				}
			  },
			  "delete": {
				"min_age": "30d",
				"actions": {
				  "delete": {
					"delete_searchable_snapshot": true
				  }
				}
			  }
			}
		  }
		}
	  }`

	want := `{
		  "policy": {
			"phases": {
			  "hot": {
				"min_age": "0ms",
				"actions": {
				  "rollover": {
					"max_size": "25GB"
				  }
				}
			  },
			  "delete": {
				"min_age": "30d",
				"actions": {
				  "delete": {
					"delete_searchable_snapshot": true
				  }
				}
			  }
			}
		  }
		}
	  `
	url := "/_ilm/policy/my-data-stream-policy"

	got, err := removeNameWrapper(url, []byte(body))
	require.NoError(t, err)
	opts := jsondiff.DefaultJSONOptions()
	diff, s := jsondiff.Compare([]byte(want), []byte(got), &opts)
	assert.Equal(t, jsondiff.SupersetMatch, diff, "want:\n%s\ngot:\n%s\ndifferences:\n%s", want, got, s)
}

func Test_removeArrayWrapper2(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		request  string
		response string
	}{
		{
			name: "index template",
			url:  "/_index_template/my-data-stream-template",
			request: `{
				"index_patterns": [ "my-data-stream*" ],
				"data_stream": { },
				"priority": 200,
				"template": {
				  "settings": {
					"index.lifecycle.name": "my-data-stream-policy"
				  }
				}
			  }`,
			response: `{
				"index_templates": [
				  {
					"name": "my-data-stream-template",
					"index_template": {
					  "index_patterns": [
						"my-data-stream*"
					  ],
					  "template": {
						"settings": {
						  "index": {
							"lifecycle": {
							  "name": "my-data-stream-policy"
							}
						  }
						}
					  },
					  "composed_of": [],
					  "priority": 200,
					  "data_stream": {}
					}
				  }
				]
			  }`,
		},
	}
	opts := jsondiff.DefaultJSONOptions()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformedResp, err := removeArrayWrapper(tt.url, []byte(tt.response))
			require.NoError(t, err)
			actual, diff := jsondiff.Compare(transformedResp, []byte(tt.request), &opts)
			assert.Equal(t, jsondiff.SupersetMatch, actual, "response:\n%s\nrequest:\n%s\ndifferences:\n%s", transformedResp, tt.request, diff)
		})
	}
}

func Test_jsondiffCompare3(t *testing.T) {

	tests := []struct {
		name      string
		a         string
		b         string
		matchType jsondiff.Difference
	}{

		{
			name: "superset subobj",
			a: `
			{"a": {"b": "a"},
			"c":"d"}`,
			b: `{
				"a": {}
				}`,
			matchType: jsondiff.SupersetMatch,
		},
		{
			name: "ilm",
			a: `{
				"version": 1,
				"modified_date": "2020-10-22T23:08:18.710Z",
				"policy": {
				  "phases": {
					"hot": {
					  "min_age": "0ms",
					  "actions": {
						"rollover": {
						  "max_size": "25GB"
						}
					  }
					},
					"delete": {
					  "min_age": "30d",
					  "actions": {
						"delete": {
						  "delete_searchable_snapshot": true
						}
					  }
					}
				  }
				}
			  }`,
			b: `{
				"policy": {
				  "phases": {
					"hot": {
					  "min_age": "0ms",
					  "actions": {
						"rollover": {
						  "max_size": "25GB"
						}
					  }
					},
					"delete": {
					  "min_age": "30d",
					  "actions": {
						"delete": {
						  "delete_searchable_snapshot": true
						}
					  }
					}
				  }
				}
			  }
			`,
			matchType: jsondiff.SupersetMatch,
		},
		{
			name: "ilm transformed",
			a: `{
				"version": 1,
				"modified_date": "2020-10-22T23:08:18.710Z",
				"policy": {
				  "phases": {
					"hot": {
					  "min_age": "0ms",
					  "actions": {
						"rollover": {
						  "max_size": "25GB"
						}
					  }
					},
					"delete": {
					  "min_age": "30d",
					  "actions": {
						"delete": {
						  "delete_searchable_snapshot": true
						}
					  }
					}
				  }
				}
			  }`,
			b: `{
				"policy": {
				  "phases": {
					"hot": {
					  "min_age": "0ms",
					  "actions": {
						"rollover": {
						  "max_size": "25GB"
						}
					  }
					},
					"delete": {
					  "min_age": "30d",
					  "actions": {
						"delete": {
						  "delete_searchable_snapshot": true
						}
					  }
					}
				  }
				}
			  }
			`,
			matchType: jsondiff.SupersetMatch,
		},
	}
	opts := jsondiff.DefaultJSONOptions()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			actual, diff := jsondiff.Compare([]byte(tt.a), []byte(tt.b), &opts)
			assert.Equal(t, tt.matchType, actual, "differences:\n%s", diff)
		})
	}

}
