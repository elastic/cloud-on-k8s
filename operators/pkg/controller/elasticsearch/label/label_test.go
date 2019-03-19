// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package label

import (
	"reflect"
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/utils/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
)

func TestClusterFromResourceLabels(t *testing.T) {
	// test when label is not set
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "name",
			Namespace: "namespace",
		},
	}
	accessor, err := meta.Accessor(&pod)
	require.NoError(t, err)
	_, exists := ClusterFromResourceLabels(accessor)
	require.False(t, exists)

	// test when label is set
	pod.ObjectMeta.Labels = map[string]string{ClusterNameLabelName: "clusterName"}
	cluster, exists := ClusterFromResourceLabels(accessor)
	require.True(t, exists)
	require.Equal(t, types.NamespacedName{
		Namespace: "namespace",
		Name:      "clusterName",
	}, cluster)
}

func TestNewLabelSelectorForElasticsearch(t *testing.T) {
	type args struct {
		es v1alpha1.Elasticsearch
	}
	tests := []struct {
		name       string
		args       args
		assertions func(*testing.T, args, labels.Selector)
	}{
		{
			name: "should match labels from NewLabels",
			args: args{es: v1alpha1.Elasticsearch{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}},
			assertions: func(t *testing.T, a args, sel labels.Selector) {
				esLabels := NewLabels(k8s.ExtractNamespacedName(&a.es))
				assert.True(t, sel.Matches(labels.Set(esLabels)))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewLabelSelectorForElasticsearch(tt.args.es)
			tt.assertions(t, tt.args, got)
		})
	}
}

func TestExtractVersion(t *testing.T) {
	tests := []struct {
		name    string
		args    corev1.Pod
		want    *version.Version
		wantErr bool
	}{
		{
			name:    "no version",
			args:    corev1.Pod{},
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid version",
			args: corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						VersionLabelName: "no a version",
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "valid version",
			args: corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Labels: map[string]string{
						VersionLabelName: "1.0.0",
					},
				},
			},
			want: &version.Version{
				Major: 1,
				Minor: 0,
				Patch: 0,
				Label: "",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractVersion(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}
