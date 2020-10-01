// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package esconfig

import (
	"io/ioutil"
	"net/http"
	"net/url"
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
					Body:       ioutil.NopCloser(strings.NewReader(`"b": "a"`)),
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
			name:    "error response from server",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := version.From(7, 9, 1)
			client := esclient.NewMockClient(v, tt.fn)
			ctx := common.NewMockContext()
			testURL, _ := url.Parse(tt.url)
			actual, err := updateRequired(ctx, client, testURL, []byte(tt.body))
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
		want      bool
		fn        esclient.RoundTripFunc
		url       string
		body      string
		wantErr   bool
	}{}
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
