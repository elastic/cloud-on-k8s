// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package operator

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNamespaceFilter_ShouldManage(t *testing.T) {
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
	filter, err := NewNamespaceFilter(selector, nil, []string{"prod-a"})
	require.NoError(t, err)

	require.True(t, filter.ShouldManage("prod-a"))
	require.False(t, filter.ShouldManage("dev-a"))
}

func TestNamespaceFilter_OnNamespaceUpsert(t *testing.T) {
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
	filter, err := NewNamespaceFilter(selector, nil, []string{"prod-a"})
	require.NoError(t, err)

	filter.OnNamespaceUpsert(corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod-b", Labels: map[string]string{"env": "prod"}}})
	filter.OnNamespaceUpsert(corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod-a", Labels: map[string]string{"env": "dev"}}})

	require.True(t, filter.ShouldManage("prod-b"))
	require.False(t, filter.ShouldManage("prod-a"))
}

func TestNamespaceFilter_OnNamespaceDelete(t *testing.T) {
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
	filter, err := NewNamespaceFilter(selector, nil, []string{"prod-a"})
	require.NoError(t, err)

	filter.OnNamespaceDelete("prod-a")

	require.False(t, filter.ShouldManage("prod-a"))
}

func TestNamespaceFilter_RespectsConfiguredNamespaces(t *testing.T) {
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}
	filter, err := NewNamespaceFilter(selector, []string{"prod-a"}, []string{"prod-a"})
	require.NoError(t, err)

	filter.OnNamespaceUpsert(corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod-b", Labels: map[string]string{"env": "prod"}}})
	filter.OnNamespaceUpsert(corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod-a", Labels: map[string]string{"env": "prod"}}})

	require.False(t, filter.ShouldManage("prod-b"))
	require.True(t, filter.ShouldManage("prod-a"))
}
