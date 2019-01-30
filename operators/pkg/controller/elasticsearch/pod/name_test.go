// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package pod

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewNodeName(t *testing.T) {
	type args struct {
		clusterName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Generates a random name from a short elasticsearch name",
			args: args{
				clusterName: "some-es-name",
			},
			want: "some-es-name-es-(.*)",
		},
		{
			name: "Generates a random name from a long elasticsearch name",
			args: args{
				clusterName: "some-es-name-that-is-quite-long-and-will-be-trimmed",
			},
			want: "some-es-name-that-is-quite-long-and-will-be-trimm-es-(.*)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewNodeName(tt.args.clusterName)
			if len(tt.args.clusterName) > maxPrefixLength {
				assert.Len(t, got, 63, got, "should be maximum 63 characters long")
			}

			assert.Regexp(t, tt.want, got)
		})
	}
}
