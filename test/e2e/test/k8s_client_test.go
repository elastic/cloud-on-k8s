// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestIsRetryableError(t *testing.T) {
	transientErrors := []error{
		apierrors.NewInternalError(errors.New("internal error")),
		apierrors.NewServerTimeout(schema.GroupResource{Resource: "agents"}, "get", 1),
		apierrors.NewServiceUnavailable("service unavailable"),
		apierrors.NewTooManyRequests("too many requests", 1),
		apierrors.NewTimeoutError("timeout", 1),
	}
	for _, err := range transientErrors {
		require.True(t, IsRetryableError(err), "expected %T to be retryable", err)
	}

	permanentErrors := []error{
		apierrors.NewBadRequest("bad request"),
		apierrors.NewForbidden(schema.GroupResource{Resource: "agents"}, "agent", errors.New("forbidden")),
		apierrors.NewNotFound(schema.GroupResource{Resource: "agents"}, "agent"),
	}
	for _, err := range permanentErrors {
		require.False(t, IsRetryableError(err), "expected %T not to be retryable", err)
	}
}

func TestCreateWithRetryAcceptsExistingObjects(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "config",
			Namespace: "default",
		},
		Data: map[string]string{"value": "old"},
	}
	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()
	k := &K8sClient{Client: baseClient}

	desiredExisting := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      existing.Name,
			Namespace: existing.Namespace,
		},
		Data: map[string]string{"value": "new"},
	}
	desiredNew := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new-config",
			Namespace: existing.Namespace,
		},
		Data: map[string]string{"value": "new"},
	}
	require.NoError(t, k.CreateWithRetry(
		func(error) bool { return false },
		time.Second,
		desiredExisting,
		desiredNew,
	))

	var actualExisting corev1.ConfigMap
	require.NoError(t, baseClient.Get(context.Background(), k8sclient.ObjectKeyFromObject(existing), &actualExisting))
	require.Equal(t, existing.Data, actualExisting.Data)

	var actualNew corev1.ConfigMap
	require.NoError(t, baseClient.Get(context.Background(), k8sclient.ObjectKeyFromObject(desiredNew), &actualNew))
	require.Equal(t, desiredNew.Data, actualNew.Data)

	require.Empty(t, desiredExisting.ResourceVersion)
	require.Empty(t, desiredNew.ResourceVersion)
}
