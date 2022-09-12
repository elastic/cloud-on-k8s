// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package tracing

import (
	"net/http"
	"net/url"
	"testing"
)

func TestRequestName(t *testing.T) {
	type args struct {
		request *http.Request
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Be nil safe",
			args: args{},
			want: "",
		},
		{
			name: "Join method and path",
			args: args{
				request: &http.Request{Method: "GET", URL: &url.URL{Path: "/a/path"}},
			},
			want: "GET /a/path",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequestName(tt.args.request); got != tt.want {
				t.Errorf("RequestName() = %v, want %v", got, tt.want)
			}
		})
	}
}
