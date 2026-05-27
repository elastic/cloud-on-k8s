// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nsmatch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// mockCache is a testify mock for cache.Cache. Only Get is set up with
// expectations; all other methods are no-op stubs required to satisfy the interface.
type mockCache struct {
	mock.Mock
}

func newMockCache(t *testing.T) *mockCache {
	t.Helper()
	m := &mockCache{}
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

var _ cache.Cache = (*mockCache)(nil)

func (m *mockCache) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	args := m.Called(key, obj)
	return args.Error(0)
}

func (m *mockCache) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return nil
}

func (m *mockCache) GetInformer(_ context.Context, _ client.Object, _ ...cache.InformerGetOption) (cache.Informer, error) {
	return nil, nil
}

func (m *mockCache) GetInformerForKind(_ context.Context, _ schema.GroupVersionKind, _ ...cache.InformerGetOption) (cache.Informer, error) {
	return nil, nil
}

func (m *mockCache) RemoveInformer(_ context.Context, _ client.Object) error { return nil }
func (m *mockCache) Start(_ context.Context) error                           { return nil }
func (m *mockCache) WaitForCacheSync(_ context.Context) bool                 { return true }

func (m *mockCache) IndexField(_ context.Context, _ client.Object, _ string, _ client.IndexerFunc) error {
	return nil
}

// OnGetFound sets up the mock to return ns when Get is called for ns.Name.
func (m *mockCache) OnGetFound(ns *corev1.Namespace) {
	m.On("Get", client.ObjectKey{Name: ns.Name}, mock.AnythingOfType("*v1.Namespace")).
		Run(func(args mock.Arguments) {
			//nolint:forcetypeassert
			*args.Get(1).(*corev1.Namespace) = *ns
		}).
		Return(nil)
}

// OnGetNotFound sets up the mock to return a NotFound error for the given namespace name.
func (m *mockCache) OnGetNotFound(name string) {
	m.On("Get", client.ObjectKey{Name: name}, mock.AnythingOfType("*v1.Namespace")).
		Return(apierrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, name))
}
