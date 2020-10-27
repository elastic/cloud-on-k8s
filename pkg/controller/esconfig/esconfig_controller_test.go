// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package esconfig

import (
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	escv1alpha1 "github.com/elastic/cloud-on-k8s/pkg/apis/esconfig/v1alpha1"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	esclient "github.com/elastic/cloud-on-k8s/pkg/controller/elasticsearch/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// this begins with generic tests then includes actual API examples to test various responses
func Test_updateRequired(t *testing.T) {
	tests := []struct {
		name    string
		want    bool
		fn      esclient.RoundTripFunc
		url     string
		body    string
		wantErr bool
	}{
		{
			name:    "exists, no update required",
			url:     "/test",
			want:    false,
			wantErr: false,
			body:    `{}`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/test", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       ioutil.NopCloser(strings.NewReader(`{}`)),
					Request:    req,
				}
			},
		},
		{
			name:    "exists, but update required",
			url:     "/test",
			want:    true,
			wantErr: false,
			body:    `{"a": "b"}`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/test", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       ioutil.NopCloser(strings.NewReader(`{"b": "a"}`)),
					Request:    req,
				}
			},
		},
		{
			name:    "update not required but response contains additional data",
			url:     "/test",
			want:    false,
			wantErr: false,
			body:    `{"a": "b"}`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/test", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       ioutil.NopCloser(strings.NewReader(`{"a": "b", "1": "2"}`)),
					Request:    req,
				}
			},
		},
		{
			name:    "does not exist, must be created",
			url:     "/test",
			want:    true,
			wantErr: false,
			body:    `{"a": "b"}`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/test", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Request:    req,
				}
			},
		},
		{
			name:    "400 status from server",
			url:     "/test",
			want:    false,
			wantErr: true,
			body:    `{"a": "b"}`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/test", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Header:     make(http.Header),
					Request:    req,
				}
			},
		},
		{
			name:    "200 status but invalid json from server",
			url:     "/test",
			want:    false,
			wantErr: true,
			body:    `{"a": "b"}`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/test", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       ioutil.NopCloser(strings.NewReader(`!`)),
					Request:    req,
				}
			},
		},

		// begin actual API response tests
		{
			// this fails because it wraps it in an object with the name of the resource
			// https://www.elastic.co/guide/en/elasticsearch/reference/7.9/getting-started-snapshot-lifecycle-management.html
			name:    "snapshot repo does not require update",
			url:     "/_snapshot/my_repository",
			want:    false,
			wantErr: false,
			body: `{
				"type": "fs",
				"settings": {
				  "location": "/tmp"
				}
			  }`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/_snapshot/my_repository", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: ioutil.NopCloser(strings.NewReader(`{
						"my_repository": {
						  "type": "fs",
						  "settings": {
							"location": "/tmp"
						  }
						}
					  }`)),
					Request: req,
				}
			},
		},

		{
			// this fails because it wraps it in an object with the name of the resource
			// https://www.elastic.co/guide/en/elasticsearch/reference/7.9/getting-started-snapshot-lifecycle-management.html
			name:    "SLM does not require update",
			url:     "/_slm/policy/nightly-snapshots",
			want:    false,
			wantErr: false,
			body: `{
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
			  }`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/_slm/policy/nightly-snapshots", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: ioutil.NopCloser(strings.NewReader(`{
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
					  }`)),
					Request: req,
				}
			},
		},
		{
			// this fails because it always wraps it in an object with the name of the policy
			// https://www.elastic.co/guide/en/elasticsearch/reference/7.9/set-up-a-data-stream.html#create-a-data-stream-template
			name:    "ILM does not require update",
			url:     "/_ilm/policy/my-data-stream-policy",
			want:    false,
			wantErr: false,
			body: `{
				"policy": {
				  "phases": {
					"hot": {
					  "actions": {
						"rollover": {
						  "max_size": "25GB"
						}
					  }
					},
					"delete": {
					  "min_age": "30d",
					  "actions": {
						"delete": {}
					  }
					}
				  }
				}
			  }`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/_ilm/policy/my-data-stream-policy", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: ioutil.NopCloser(strings.NewReader(`{
						"my-data-stream-policy": {
						  "version": 1,
						  "modified_date": "2020-10-22T23:08:18.710Z",
						  "policy": {
							"phases": {
							  "hot": {
								"min_age": "0ms",
								"actions": {
								  "rollover": {
									"max_size": "25gb"
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
					  }`)),
					Request: req,
				}
			},
		},
		{
			// https://www.elastic.co/guide/en/elasticsearch/reference/7.9/set-up-a-data-stream.html#create-a-data-stream-template
			// this works because if the body is empty we do not compare responses
			name:    "datastream does not require update",
			url:     "/_data_stream/my-data-stream-alt",
			want:    false,
			wantErr: false,
			// TODO we may want to either require an empty body in the openapi spec or default it internally to `{}` as our parser fails if there is nothing
			body: `{}`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/_data_stream/my-data-stream-alt", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: ioutil.NopCloser(strings.NewReader(`{
						"data_streams": [
						  {
							"name": "my-data-stream-alt",
							"timestamp_field": {
							  "name": "@timestamp"
							},
							"indices": [
							  {
								"index_name": ".ds-my-data-stream-alt-000001",
								"index_uuid": "gl-oxiU7QZKUSpNoxegHYA"
							  }
							],
							"generation": 1,
							"status": "YELLOW",
							"template": "my-data-stream-template",
							"ilm_policy": "my-data-stream-policy"
						  }
						]
					  }`)),
					Request: req,
				}
			},
		},
		{
			// https://www.elastic.co/guide/en/elasticsearch/reference/7.9/set-up-a-data-stream.html#create-a-data-stream-template
			// this fails because it a) always returns an index_templates array and then includes our setting in an index_template object
			name:    "index template does not require update",
			url:     "/_index_template/my-data-stream-template",
			want:    false,
			wantErr: false,
			body: `{
				"index_patterns": [ "my-data-stream*" ],
				"data_stream": { },
				"priority": 200,
				"template": {
				  "settings": {
					"index.lifecycle.name": "my-data-stream-policy"
				  }
				}
			  }`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/_index_template/my-data-stream-template", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: ioutil.NopCloser(strings.NewReader(`{
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
					  }`)),
					Request: req,
				}
			},
		},

		{
			// https://www.elastic.co/guide/en/elasticsearch/reference/7.9/put-pipeline-api.html
			// this fails because it always wraps it in an object with the name of the policy
			name:    "ingest pipeline does not require update",
			url:     "/_ingest/pipeline/my-pipeline-id",
			want:    false,
			wantErr: false,
			body: `{
				"description" : "describe pipeline",
				"processors" : [
				  {
					"set" : {
					  "field": "foo",
					  "value": "bar"
					}
				  }
				]
			  }`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/_ingest/pipeline/my-pipeline-id", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: ioutil.NopCloser(strings.NewReader(`{
						"my-pipeline-id": {
						  "description": "describe pipeline",
						  "processors": [
							{
							  "set": {
								"field": "foo",
								"value": "bar"
							  }
							}
						  ]
						}
					  }`)),
					Request: req,
				}
			},
		},
		{
			// https://www.elastic.co/guide/en/elasticsearch/reference/7.9/indices-create-index.html
			// https://www.elastic.co/guide/en/elasticsearch/reference/7.9/indices-get-index.html
			// this works because the user specified no body, if they did specify something it would fail because it wraps the response
			// in an object with the index name
			name:    "index with no body does not require update",
			url:     "/my-index-000001",
			want:    false,
			wantErr: false,
			body:    `{}`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/my-index-000001", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: ioutil.NopCloser(strings.NewReader(`{
						"my-index-000001": {
						  "aliases": {},
						  "mappings": {},
						  "settings": {
							"index": {
							  "creation_date": "1603409863722",
							  "number_of_shards": "1",
							  "number_of_replicas": "1",
							  "uuid": "5m59cAYNQCOhbEzH68gzEA",
							  "version": {
								"created": "7090199"
							  },
							  "provided_name": "my-index-000001"
							}
						  }
						}
					  }`)),
					Request: req,
				}
			},
		},

		{
			// TODO open docs PR to change this to create or PUT rather than Add in the URL
			// TODO open docs PR to correct that the filter is optional not required
			// https://www.elastic.co/guide/en/elasticsearch/reference/7.9/indices-add-alias.html
			// https://www.elastic.co/guide/en/elasticsearch/reference/7.9/indices-get-alias.html
			// this works because the user specified no body
			name:    "index alias with no body does not require update",
			url:     "/my-index-000001/_alias/alias1",
			want:    false,
			wantErr: false,
			body:    `{}`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/my-index-000001/_alias/alias1", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: ioutil.NopCloser(strings.NewReader(`{
						"my-index-000001": {
						  "aliases": {
							"alias1": {}
						  }
						}
					  }`)),
					Request: req,
				}
			},
		},
		{
			// https://www.elastic.co/guide/en/elasticsearch/reference/7.9/indices-create-index.html
			// https://www.elastic.co/guide/en/elasticsearch/reference/7.9/indices-get-index.html
			// this fails because it wraps the response in an object with the index name
			name:    "index with body does not require update",
			url:     "/users",
			want:    false,
			wantErr: false,
			body: `{
				"mappings" : {
				  "properties" : {
					"user_id" : {"type" : "integer"}
				  }
				}
			  }`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/users", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: ioutil.NopCloser(strings.NewReader(`{
						"users": {
						  "aliases": {},
						  "mappings": {
							"properties": {
							  "user_id": {
								"type": "integer"
							  }
							}
						  },
						  "settings": {
							"index": {
							  "creation_date": "1603410376675",
							  "number_of_shards": "1",
							  "number_of_replicas": "1",
							  "uuid": "24Cc709UTAaYTn9u1xpCOA",
							  "version": {
								"created": "7090199"
							  },
							  "provided_name": "users"
							}
						  }
						}
					  }`)),
					Request: req,
				}
			},
		},
		{
			// https://www.elastic.co/guide/en/elasticsearch/reference/7.9/indices-add-alias.html
			// https://www.elastic.co/guide/en/elasticsearch/reference/7.9/indices-get-alias.html
			// this fails because it wraps the response in an object with the alias name
			name:    "index alias with body does not require update",
			url:     "/users/_alias/user_12",
			want:    false,
			wantErr: false,
			body: `{
				"routing" : "12",
				"filter" : {
				  "term" : {
					"user_id" : 12
				  }
				}
			  }`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/users/_alias/user_12", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: ioutil.NopCloser(strings.NewReader(`{
						"users": {
						  "aliases": {
							"user_12": {
							  "filter": {
								"term": {
								  "user_id": 12
								}
							  },
							  "index_routing": "12",
							  "search_routing": "12"
							}
						  }
						}
					  }`)),
					Request: req,
				}
			},
		},
		{
			// https://www.elastic.co/guide/en/elasticsearch/reference/7.9/index-templates.html
			// this fails because it wraps the response in an object with the object name
			name:    "component template does not require update",
			url:     "/_component_template/component_template1",
			want:    false,
			wantErr: false,
			body: `{
				"template": {
				  "mappings": {
					"properties": {
					  "@timestamp": {
						"type": "date"
					  }
					}
				  }
				}
			  }`,
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/_component_template/component_template1", req.URL.Path)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body: ioutil.NopCloser(strings.NewReader(`{
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
					  }`)),
					Request: req,
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := version.From(7, 9, 1)
			client := esclient.NewMockClient(v, tt.fn)
			ctx := common.NewMockContext()
			actual, err := updateRequired(ctx, client, tt.url, []byte(tt.body))
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want, actual)
		})
	}
}

func TestReconcileOperation(t *testing.T) {
	tests := []struct {
		name      string
		operation escv1alpha1.ElasticsearchConfigOperation
		fn        esclient.RoundTripFunc
		wantErr   bool
	}{
		{
			name: "no updates required",
			operation: escv1alpha1.ElasticsearchConfigOperation{
				URL:  "/test",
				Body: `{"a": "b"}`,
			},
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/test", req.URL.Path)
				// should be no PUTs in this instance
				require.Equal(t, http.MethodGet, req.Method)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       ioutil.NopCloser(strings.NewReader(`{"a": "b"}`)),
					Request:    req,
				}
			},
			wantErr: false,
		},
		{
			name: "updates required, no error",
			operation: escv1alpha1.ElasticsearchConfigOperation{
				URL:  "/test",
				Body: `{"a": "b"}`,
			},
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/test", req.URL.Path)
				if req.Method == http.MethodGet {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       ioutil.NopCloser(strings.NewReader(`{"1": "2"}`)),
						Request:    req,
					}
				}
				require.Equal(t, http.MethodPut, req.Method)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Request:    req,
				}
			},
			wantErr: false,
		},
		{
			name: "updates required, PUT errors out",
			operation: escv1alpha1.ElasticsearchConfigOperation{
				URL:  "/test",
				Body: `{"a": "b"}`,
			},
			fn: func(req *http.Request) *http.Response {
				require.Equal(t, "/test", req.URL.Path)
				if req.Method == http.MethodGet {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       ioutil.NopCloser(strings.NewReader(`{"1": "2"}`)),
						Request:    req,
					}
				}
				require.Equal(t, http.MethodPut, req.Method)
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Header:     make(http.Header),
					Request:    req,
				}
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := version.From(7, 9, 1)
			client := esclient.NewMockClient(v, tt.fn)
			ctx := common.NewMockContext()
			err := ReconcileOperation(ctx, client, tt.operation)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
