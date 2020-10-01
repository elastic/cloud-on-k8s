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
