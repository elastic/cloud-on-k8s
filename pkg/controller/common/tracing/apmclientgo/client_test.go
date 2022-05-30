// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package apmclientgo

import (
	"net/http"
	"testing"
)

func Test_requestName(t *testing.T) {
	type args struct {
		req *http.Request
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "WATCH request",
			args: args{
				req: mustRequest("GET", "https://34.76.65.60/apis/elasticsearch.k8s.elastic.co/v1/elasticsearches?allowWatchBookmarks=true&resourceVersion=11980723&timeout=1m0s&timeoutSeconds=526&watch=true"),
			},
			want: "WATCH v1/elasticsearches",
		},
		{
			name: "PUT request",
			args: args{
				req: mustRequest("PUT", "https://34.76.65.60/api/v1/namespaces/default/services/elasticsearch-es-transport?timeout=1m0s"),
			},
			want: "PUT default/services/elasticsearch-es-transport", // no version on single resources requests, but maybe namespace?
		},
		{
			name: "list request",
			args: args{
				req: mustRequest("GET", "https://34.76.65.60/apis/policy/v1beta1/poddisruptionbudgets?limit=500&resourceVersion=0&timeout=1m0s"),
			},
			want: "GET v1beta1/poddisruptionbudgets",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requestName(tt.args.req); got != tt.want {
				t.Errorf("requestName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func mustRequest(method, url string) *http.Request {
	request, err := http.NewRequest(method, url, nil) //nolint:noctx
	if err != nil {
		panic(err)
	}
	return request
}
