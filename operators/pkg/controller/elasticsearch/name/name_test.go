// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package name

import (
	"fmt"
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
			if len(got) > MaxLabelLength {
				assert.Len(t, got, MaxLabelLength,
					got, fmt.Sprintf("should be maximum %d characters long", MaxLabelLength))
			}

			assert.Regexp(t, tt.want, got)
		})
	}
}

func TestNewPVCName(t *testing.T) {
	type args struct {
		podName         string
		pvcTemplateName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Generates a random name from a short pvc template name",
			args: args{
				podName:         "some-es-name-xxxxxxxxx-es-2qnjmqsv4s",
				pvcTemplateName: "a-pvc-name",
			},
			want: "some-es-name-xxxxxxxxx-es-2qnjmqsv4s-a-pvc-name",
		},
		{
			name: "Generates a random name from a long pod name",
			args: args{
				podName:         "some-es-name-that-is-quite-long-and-will-be-trimmed-es-2qnjmqsv4s",
				pvcTemplateName: "a-pvc-name",
			},
			want: "some-es-name-that-is-quite-long-and--a-pvc-name",
		},
		{
			name: "Generates a random name from a long pvc template name",
			args: args{
				podName:         "some-es-name-xxxxxxxxx-es-2qnjmqsv4s",
				pvcTemplateName: "some-pvc-name-that-is-quite-loooooong",
			},
			want: "some-es-name-xxxxxxxxx-es-2qnjmqsv4s-some-pvc-name-that-is-quit",
		},
		{
			name: "Generates a random name from a long pod name",
			args: args{
				podName:         "some-es-name-that-is-quite-long-and-will-be-trimmed-es-2qnjmqsv4s",
				pvcTemplateName: "some-pvc-name-that-is-quite-loooooong",
			},
			want: "some-es-name-that-is-quite-long-and--some-pvc-name-that-is-quit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewPVCName(tt.args.podName, tt.args.pvcTemplateName)
			if len(got) > MaxLabelLength {
				assert.Len(t, got, MaxLabelLength,
					got, fmt.Sprintf("should be maximum %d characters long", MaxLabelLength))
			}

			assert.Equal(t, tt.want, got)
		})
	}
}
