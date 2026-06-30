// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package mock

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Cache stubs List and Get on cache.Cache via testify/mock.
type Cache struct {
	mock.Mock
	cache.Cache
}

func (m *Cache) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	args := m.Called(ctx, key, obj, opts)
	return args.Error(0)
}

func (m *Cache) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	args := m.Called(ctx, list, opts)
	return args.Error(0)
}

// OnListSetNamespaceList sets up a List expectation that populates the NamespaceList with namespaces.
func (m *Cache) OnListSetNamespaceList(namespaces ...corev1.Namespace) *mock.Call {
	return m.On("List", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			args.Get(1).(*corev1.NamespaceList).Items = namespaces //nolint:forcetypeassert
		})
}

// OnGetSetNamespace sets up a Get expectation that populates obj as a Namespace with the given labels.
func (m *Cache) OnGetSetNamespace(lbls map[string]string) *mock.Call {
	return m.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			key := args.Get(1).(client.ObjectKey) //nolint:forcetypeassert
			ns := args.Get(2).(*corev1.Namespace) //nolint:forcetypeassert
			ns.Name = key.Name
			ns.Labels = lbls
			ns.ObjectMeta = metav1.ObjectMeta{Name: key.Name, Labels: lbls}
		})
}

// newMockCache creates a mockCache and registers AssertExpectations as a test cleanup.
func NewCache(t *testing.T) *Cache {
	t.Helper()
	m := &Cache{}
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

// Client stubs List and Get on client.Client via testify/mock.
type Client struct {
	mock.Mock
	client.Client
}

func (m *Client) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	args := m.Called(ctx, list, opts)
	return args.Error(0)
}

func (m *Client) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	args := m.Called(ctx, key, obj, opts)
	return args.Error(0)
}

// OnListSetPodList sets up a List expectation that populates the PodList with pods.
func (m *Client) OnListSetPodList(pods ...corev1.Pod) *mock.Call {
	return m.On("List", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			args.Get(1).(*corev1.PodList).Items = pods //nolint:forcetypeassert
		})
}

// OnGetSetPod sets up a Get expectation that populates obj as a Pod in ns.
func (m *Client) OnGetSetPod() *mock.Call {
	return m.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			key := args.Get(1).(types.NamespacedName) //nolint:forcetypeassert
			p := args.Get(2).(*corev1.Pod)            //nolint:forcetypeassert
			p.Name = key.Name
			p.Namespace = key.Namespace
		})
}

// NewClient creates a mockClient and registers AssertExpectations as a test cleanup.
func NewClient(t *testing.T) *Client {
	t.Helper()
	m := new(Client)
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}
