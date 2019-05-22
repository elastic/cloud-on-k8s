// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package name

import (
	"fmt"
	"testing"

	"github.com/elastic/cloud-on-k8s/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var es = v1alpha1.Elasticsearch{
	ObjectMeta: v1.ObjectMeta{Name: "elasticsearch"},
}

func TestNewNodeName(t *testing.T) {
	type args struct {
		clusterName string
		nodeSpec    v1alpha1.NodeSpec
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
		{
			name: "Generates a random name from a short elasticsearch name with a nodeSpec.Name",
			args: args{
				clusterName: "some-es-name",
				nodeSpec: v1alpha1.NodeSpec{
					Name: "foo",
				},
			},
			want: "some-es-name-es-foo-(.*)",
		},
		{
			name: "Generates a random name from a long elasticsearch name with a nodeSpec.Name",
			args: args{
				clusterName: "some-es-name-that-is-quite-long-and-will-be-trimmed",
				nodeSpec: v1alpha1.NodeSpec{
					Name: "foo",
				},
			},
			want: "some-es-name-that-is-quite-long-and-will-be-t-es-foo-(.*)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewPodName(tt.args.clusterName, tt.args.nodeSpec)
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
			name: "Generates a random name from a long pod name (should not happen)",
			args: args{
				podName:         "some-es-name-that-is-quite-long-and-will-be-trimmed-es-2qnjmqsv4s",
				pvcTemplateName: "a-pvc-name",
			},
			want: "some-es-name-that-is-quite-long-and-will-be-trimmed--a-pvc-name",
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
			name: "Generates a random name from a long pod name (should not happen) and a long pvc template name",
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

func TestBasename(t *testing.T) {
	type args struct {
		podName string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "pod name with no segments",
			args: args{
				podName: "foo",
			},
			want: "foo",
		},
		{
			name: "sample pod name",
			args: args{
				podName: "sample-1-es-mqjcddtv6g",
			},
			want: "sample-1-es",
		},
		{
			name: "sample pod name with nodespec name",
			args: args{
				podName: "sample-1-es-foo-mqjcddtv6g",
			},
			want: "sample-1-es-foo",
		},
		{
			name: "new pod",
			args: args{
				podName: NewPodName(es.Name, v1alpha1.NodeSpec{}),
			},
			want: "elasticsearch-es",
		},
		{
			name: "new pod with nodespec name",
			args: args{
				podName: NewPodName(es.Name, v1alpha1.NodeSpec{Name: "foo"}),
			},
			want: "elasticsearch-es-foo",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Basename(tt.args.podName); got != tt.want {
				t.Errorf("Basename() = %v, want %v", got, tt.want)
			}
		})
	}
}
