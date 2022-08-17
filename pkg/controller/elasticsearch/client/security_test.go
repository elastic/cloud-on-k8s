// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package client

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/elastic/cloud-on-k8s/v2/pkg/controller/common/version"
)

func Test_GetServiceAccountCredentials(t *testing.T) {
	type args struct {
		namespacedService string
	}
	tests := []struct {
		name    string
		client  Client
		args    args
		want    ServiceAccountCredential
		wantErr bool
	}{
		{
			args: args{namespacedService: "elastic/kibana"},
			want: ServiceAccountCredential{NodesCredentials: NodesCredentials{
				FileTokens: map[string]FileToken{
					"default_kibana-sample_50d2f2d2-d989-4ab7-a3d4-c9e31e5651ca": {Nodes: []string{"elasticsearch-sample-es-default-0"}},
				}},
			},
			client: NewMockClient(version.MustParse("7.17.0"), func(req *http.Request) *http.Response {
				require.Equal(t, "/_security/service/elastic/kibana/credential", req.URL.Path)
				return &http.Response{
					StatusCode: 200,
					Body: io.NopCloser(strings.NewReader(
						`{
	"service_account": "elastic/kibana",
	"count": 1,
	"tokens": {},
	"nodes_credentials": {
		"_nodes": {
			"total": 1,
			"successful": 1,
			"failed": 0
		},
		"file_tokens": {
			"default_kibana-sample_50d2f2d2-d989-4ab7-a3d4-c9e31e5651ca": {
				"nodes": ["elasticsearch-sample-es-default-0"]
			}
		}
	}
}`)),
					Header:  make(http.Header),
					Request: req,
				}
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.client.GetServiceAccountCredentials(context.TODO(), tt.args.namespacedService)
			if (err != nil) != tt.wantErr {
				t.Errorf("clientV7.GetServiceAccountCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("client.GetServiceAccountCredentials() = %v, want %v", got, tt.want)
			}
		})
	}
}
