// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package annotation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/elastic/cloud-on-k8s/v3/pkg/utils/k8s"
)

func TestHasClientAuthenticationRequired(t *testing.T) {
	t.Run("returns true when annotation is set to true", func(t *testing.T) {
		obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{ClientAuthenticationRequiredAnnotation: "true"},
		}}
		require.True(t, HasClientAuthenticationRequired(obj))
	})

	t.Run("returns false when annotation is absent", func(t *testing.T) {
		obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{}}
		require.False(t, HasClientAuthenticationRequired(obj))
	})

	t.Run("returns false when annotation has unexpected value", func(t *testing.T) {
		obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{ClientAuthenticationRequiredAnnotation: "false"},
		}}
		require.False(t, HasClientAuthenticationRequired(obj))
	})

	t.Run("returns false when annotation is empty string", func(t *testing.T) {
		obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{ClientAuthenticationRequiredAnnotation: ""},
		}}
		require.False(t, HasClientAuthenticationRequired(obj))
	})
}

func TestSetClientAuthenticationRequiredAnnotation(t *testing.T) {
	ctx := context.Background()
	nsn := types.NamespacedName{Namespace: "ns", Name: "test"}

	t.Run("sets annotation on object with no annotations", func(t *testing.T) {
		obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "test"}}
		client := k8s.NewFakeClient(obj)

		require.NoError(t, SetClientAuthenticationRequiredAnnotation(ctx, client, obj))

		var updated corev1.Secret
		require.NoError(t, client.Get(ctx, nsn, &updated))
		require.Equal(t, "true", updated.Annotations[ClientAuthenticationRequiredAnnotation])
	})

	t.Run("no-op when annotation already present", func(t *testing.T) {
		obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns",
			Name:        "test",
			Annotations: map[string]string{ClientAuthenticationRequiredAnnotation: "true"},
		}}
		client := k8s.NewFakeClient(obj)

		require.NoError(t, SetClientAuthenticationRequiredAnnotation(ctx, client, obj))

		var updated corev1.Secret
		require.NoError(t, client.Get(ctx, nsn, &updated))
		require.Equal(t, "true", updated.Annotations[ClientAuthenticationRequiredAnnotation])
	})

	t.Run("overwrites incorrect annotation value", func(t *testing.T) {
		obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns",
			Name:        "test",
			Annotations: map[string]string{ClientAuthenticationRequiredAnnotation: "false"},
		}}
		client := k8s.NewFakeClient(obj)

		require.NoError(t, SetClientAuthenticationRequiredAnnotation(ctx, client, obj))

		var updated corev1.Secret
		require.NoError(t, client.Get(ctx, nsn, &updated))
		require.Equal(t, "true", updated.Annotations[ClientAuthenticationRequiredAnnotation])
	})
}

func TestRemoveClientAuthenticationRequiredAnnotation(t *testing.T) {
	ctx := context.Background()
	nsn := types.NamespacedName{Namespace: "ns", Name: "test"}

	t.Run("removes annotation when present", func(t *testing.T) {
		obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Namespace:   "ns",
			Name:        "test",
			Annotations: map[string]string{ClientAuthenticationRequiredAnnotation: "true"},
		}}
		client := k8s.NewFakeClient(obj)

		require.NoError(t, RemoveClientAuthenticationRequiredAnnotation(ctx, client, obj))

		var updated corev1.Secret
		require.NoError(t, client.Get(ctx, nsn, &updated))
		require.False(t, HasClientAuthenticationRequired(&updated))
	})

	t.Run("no-op when annotation already absent", func(t *testing.T) {
		obj := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "test"}}
		client := k8s.NewFakeClient(obj)

		require.NoError(t, RemoveClientAuthenticationRequiredAnnotation(ctx, client, obj))

		var updated corev1.Secret
		require.NoError(t, client.Get(ctx, nsn, &updated))
		require.Empty(t, updated.Annotations)
	})
}
