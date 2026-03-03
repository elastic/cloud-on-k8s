// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package common

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/common/operator"
	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

type testcase struct {
	name string

	// annotationSequence is list of annotations that are simulated.
	annotationSequence []map[string]string

	// Expected (un)managed state.
	expectedState []bool
}

func TestUnmanagedCondition(t *testing.T) {
	var tests = []testcase{
		{
			name: "Simple unmanaged/managed simulation (a.k.a the Happy Path)",
			annotationSequence: []map[string]string{
				{ManagedAnnotation: "true"},
				{ManagedAnnotation: "false"},
				{ManagedAnnotation: "true"},
				{ManagedAnnotation: "false"},
			},
			expectedState: []bool{
				false,
				true,
				false,
				true,
			},
		},
		{
			name: "Anything but 'false' means managed",
			annotationSequence: []map[string]string{
				{ManagedAnnotation: ""}, // empty annotation
				{ManagedAnnotation: "false"},
				{ManagedAnnotation: "XXXX"}, // unable to parse these
				{ManagedAnnotation: "1"},
				{ManagedAnnotation: "0"},
			},
			expectedState: []bool{
				false,
				true,
				false,
				false,
				false,
			},
		},
		{
			name: "Still support legacy annotation",
			annotationSequence: []map[string]string{
				{LegacyPauseAnnoation: "true"}, // still support legacy for backwards compatibility
				{LegacyPauseAnnoation: "false"},
				{LegacyPauseAnnoation: "foo"},
				{LegacyPauseAnnoation: "false", ManagedAnnotation: "false"}, // new one takes precedence
				{LegacyPauseAnnoation: "true", ManagedAnnotation: "true"},   // but legacy is respected if true
			},
			expectedState: []bool{
				true,
				false,
				false,
				true,
				true,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for i, expectedState := range test.expectedState {
				// testing with a secret, but could be any kind
				obj := corev1.Secret{ObjectMeta: v1.ObjectMeta{
					Name:        "bar",
					Namespace:   "foo",
					Annotations: test.annotationSequence[i],
				}}
				actualPauseState := IsUnmanaged(context.Background(), &obj)
				assert.Equal(t, expectedState, actualPauseState, test.annotationSequence[i])
			}
		})
	}
}

func TestIsUnmanagedOrFiltered(t *testing.T) {
	testNamespace := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: "test-ns", Labels: map[string]string{"env": "prod"}}}
	devNamespace := &corev1.Namespace{ObjectMeta: v1.ObjectMeta{Name: "dev-ns", Labels: map[string]string{"env": "dev"}}}
	fakeClient := k8s.NewFakeClient(testNamespace, devNamespace)

	t.Run("returns unmanaged when annotation is set", func(t *testing.T) {
		obj := &corev1.Secret{ObjectMeta: v1.ObjectMeta{Name: "s1", Namespace: "test-ns", Annotations: map[string]string{ManagedAnnotation: "false"}}}

		unmanagedOrFiltered, err := IsUnmanagedOrFiltered(context.Background(), fakeClient, obj, operator.Parameters{})

		assert.NoError(t, err)
		assert.True(t, unmanagedOrFiltered)
	})

	t.Run("returns filtered when namespace does not match selector", func(t *testing.T) {
		obj := &corev1.Secret{ObjectMeta: v1.ObjectMeta{Name: "s2", Namespace: "dev-ns"}}
		params := operator.Parameters{NamespaceLabelSelector: &v1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}}

		unmanagedOrFiltered, err := IsUnmanagedOrFiltered(context.Background(), fakeClient, obj, params)

		assert.NoError(t, err)
		assert.True(t, unmanagedOrFiltered)
	})

	t.Run("returns managed when namespace matches selector", func(t *testing.T) {
		obj := &corev1.Secret{ObjectMeta: v1.ObjectMeta{Name: "s3", Namespace: "test-ns"}}
		params := operator.Parameters{NamespaceLabelSelector: &v1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}}

		unmanagedOrFiltered, err := IsUnmanagedOrFiltered(context.Background(), fakeClient, obj, params)

		assert.NoError(t, err)
		assert.False(t, unmanagedOrFiltered)
	})

	t.Run("returns error when namespace lookup fails", func(t *testing.T) {
		obj := &corev1.Secret{ObjectMeta: v1.ObjectMeta{Name: "s4", Namespace: "missing-ns"}}
		params := operator.Parameters{NamespaceLabelSelector: &v1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}}

		unmanagedOrFiltered, err := IsUnmanagedOrFiltered(context.Background(), fakeClient, obj, params)

		assert.Error(t, err)
		assert.False(t, unmanagedOrFiltered)
	})
}
