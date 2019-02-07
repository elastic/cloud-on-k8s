// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package label

import (
	"testing"

	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		es v1alpha1.ElasticsearchCluster
	}
	tests := []struct {
		name       string
		args       args
		assertions func(*testing.T, args, labels.Selector)
	}{
		{
			name: "should match labels from NewLabels",
			args: args{es: v1alpha1.ElasticsearchCluster{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}},
			assertions: func(t *testing.T, a args, sel labels.Selector) {
				esLabels := NewLabels(a.es)
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
