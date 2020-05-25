// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package kibana

import (
	"testing"
)

func TestHTTPService(t *testing.T) {
	type args struct {
		kbName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "sample",
			args: args{kbName: "sample"},
			want: "sample-kb-http",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HTTPService(tt.args.kbName); got != tt.want {
				t.Errorf("HTTPService() = %v, want %v", got, tt.want)
			}
		})
	}
}
