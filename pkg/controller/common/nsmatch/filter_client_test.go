// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nsmatch

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// mockClient stubs List and Get on client.Client via testify/mock.
type mockClient struct {
	mock.Mock
	client.Client
}

func (m *mockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	args := m.Called(ctx, list, opts)
	return args.Error(0)
}

func (m *mockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	args := m.Called(ctx, key, obj, opts)
	return args.Error(0)
}

// OnListSetPodList sets up a List expectation that populates the PodList with pods.
func (m *mockClient) OnListSetPodList(pods ...corev1.Pod) *mock.Call {
	return m.On("List", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			args.Get(1).(*corev1.PodList).Items = pods //nolint:forcetypeassert
		})
}

// OnGetSetPod sets up a Get expectation that populates obj as a Pod in ns.
func (m *mockClient) OnGetSetPod() *mock.Call {
	return m.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			key := args.Get(1).(types.NamespacedName) //nolint:forcetypeassert
			p := args.Get(2).(*corev1.Pod)            //nolint:forcetypeassert
			p.Name = key.Name
			p.Namespace = key.Namespace
		})
}

// newMockClient creates a mockClient and registers AssertExpectations as a test cleanup.
func newMockClient(t *testing.T) *mockClient {
	t.Helper()
	m := new(mockClient)
	t.Cleanup(func() { m.AssertExpectations(t) })
	return m
}

// makeFilterClient builds a FilterClient whose notifier has sel active and the given namespaces pre-seeded as matching.
func makeFilterClient(delegate client.Client, sel labels.Selector, matchedNSes ...string) *FilterClient {
	nfn := NewMatchNotifier(sel, testOperatorNS)
	for _, ns := range matchedNSes {
		nfn.Swap(ns, true)
	}
	return NewFilterClient(delegate, nfn)
}

func pod(name, ns string) corev1.Pod {
	return corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
}

func podNames(list *corev1.PodList) []string {
	names := make([]string, len(list.Items))
	for i, p := range list.Items {
		names[i] = p.Name
	}
	return names
}

func TestFilterClientList(t *testing.T) {
	sel := mustSelector(t, map[string]string{"env": "prod"})
	ctx := context.Background()

	t.Run("delegate error is propagated without filtering", func(t *testing.T) {
		m := newMockClient(t)
		m.OnListSetPodList().Return(errors.New("api server unavailable"))
		fc := NewFilterClient(m, NewMatchNotifier(sel, testOperatorNS))
		require.Error(t, fc.List(ctx, &corev1.PodList{}))
	})

	t.Run("nil notifier: all items returned unfiltered", func(t *testing.T) {
		m := newMockClient(t)
		m.OnListSetPodList(pod("a", "ns-a"), pod("b", "ns-b")).Return(nil)
		fc := NewFilterClient(m, nil)
		list := &corev1.PodList{}
		require.NoError(t, fc.List(ctx, list))
		assert.Len(t, list.Items, 2)
	})

	t.Run("selector disabled: all items returned unfiltered", func(t *testing.T) {
		m := newMockClient(t)
		m.OnListSetPodList(pod("a", "ns-a"), pod("b", "ns-b")).Return(nil)
		fc := NewFilterClient(m, NewMatchNotifier(nil, testOperatorNS))
		list := &corev1.PodList{}
		require.NoError(t, fc.List(ctx, list))
		assert.Len(t, list.Items, 2)
	})

	t.Run("namespace-scoped list, namespace matches: items unchanged", func(t *testing.T) {
		m := newMockClient(t)
		m.OnListSetPodList(pod("a", "prod-ns"), pod("b", "prod-ns")).Return(nil)
		fc := makeFilterClient(m, sel, "prod-ns")
		list := &corev1.PodList{}
		require.NoError(t, fc.List(ctx, list, client.InNamespace("prod-ns")))
		assert.Len(t, list.Items, 2)
	})

	t.Run("namespace-scoped list, namespace does not match: all items cleared", func(t *testing.T) {
		m := newMockClient(t)
		m.OnListSetPodList(pod("a", "dev-ns"), pod("b", "dev-ns")).Return(nil)
		fc := makeFilterClient(m, sel) // dev-ns not seeded
		list := &corev1.PodList{}
		require.NoError(t, fc.List(ctx, list, client.InNamespace("dev-ns")))
		assert.Empty(t, list.Items)
	})

	t.Run("cluster-scoped list, all namespaces match: all items kept", func(t *testing.T) {
		m := newMockClient(t)
		m.OnListSetPodList(pod("a", "ns-1"), pod("b", "ns-2")).Return(nil)
		fc := makeFilterClient(m, sel, "ns-1", "ns-2")
		list := &corev1.PodList{}
		require.NoError(t, fc.List(ctx, list))
		assert.ElementsMatch(t, []string{"a", "b"}, podNames(list))
	})

	t.Run("cluster-scoped list, no namespace matches: all items removed", func(t *testing.T) {
		m := newMockClient(t)
		m.OnListSetPodList(pod("a", "ns-1"), pod("b", "ns-2")).Return(nil)
		fc := makeFilterClient(m, sel) // neither seeded
		list := &corev1.PodList{}
		require.NoError(t, fc.List(ctx, list))
		assert.Empty(t, list.Items)
	})

	t.Run("cluster-scoped list, mixed namespaces: only matching items kept", func(t *testing.T) {
		m := newMockClient(t)
		m.OnListSetPodList(pod("a", "prod-ns"), pod("b", "dev-ns"), pod("c", "prod-ns")).Return(nil)
		fc := makeFilterClient(m, sel, "prod-ns")
		list := &corev1.PodList{}
		require.NoError(t, fc.List(ctx, list))
		assert.ElementsMatch(t, []string{"a", "c"}, podNames(list))
	})

	t.Run("cluster-scoped list, empty result: stays empty", func(t *testing.T) {
		m := newMockClient(t)
		m.OnListSetPodList().Return(nil)
		fc := makeFilterClient(m, sel, "prod-ns")
		list := &corev1.PodList{}
		require.NoError(t, fc.List(ctx, list))
		assert.Empty(t, list.Items)
	})

	t.Run("operator namespace always passes filter", func(t *testing.T) {
		m := newMockClient(t)
		m.OnListSetPodList(pod("op", testOperatorNS), pod("b", "dev-ns")).Return(nil)
		fc := makeFilterClient(m, sel) // no extra namespaces seeded
		list := &corev1.PodList{}
		require.NoError(t, fc.List(ctx, list))
		assert.ElementsMatch(t, []string{"op"}, podNames(list))
	})

	t.Run("empty namespace (cluster-scoped resource) always passes filter", func(t *testing.T) {
		m := newMockClient(t)
		m.OnListSetPodList(pod("cluster-res", ""), pod("b", "dev-ns")).Return(nil)
		fc := makeFilterClient(m, sel) // no extra namespaces seeded
		list := &corev1.PodList{}
		require.NoError(t, fc.List(ctx, list))
		assert.ElementsMatch(t, []string{"cluster-res"}, podNames(list))
	})
}

func TestFilterClientGet(t *testing.T) {
	sel := mustSelector(t, map[string]string{"env": "prod"})
	ctx := context.Background()

	t.Run("delegate error is propagated without filtering", func(t *testing.T) {
		m := newMockClient(t)
		m.OnGetSetPod().Return(errors.New("api server unavailable"))
		fc := NewFilterClient(m, NewMatchNotifier(sel, testOperatorNS))
		require.Error(t, fc.Get(ctx, client.ObjectKey{Name: "my-pod", Namespace: "prod-ns"}, &corev1.Pod{}))
	})

	t.Run("nil notifier: object returned unfiltered", func(t *testing.T) {
		m := newMockClient(t)
		m.OnGetSetPod().Return(nil)
		fc := NewFilterClient(m, nil)
		obj := &corev1.Pod{}
		require.NoError(t, fc.Get(ctx, types.NamespacedName{Name: "my-pod", Namespace: "dev-ns"}, obj))
		assert.Equal(t, "dev-ns", obj.Namespace)
	})

	t.Run("selector disabled: object returned unfiltered", func(t *testing.T) {
		m := newMockClient(t)
		m.OnGetSetPod().Return(nil)
		fc := NewFilterClient(m, NewMatchNotifier(nil, testOperatorNS))
		obj := &corev1.Pod{}
		require.NoError(t, fc.Get(ctx, types.NamespacedName{Name: "my-pod", Namespace: "dev-ns"}, obj))
		assert.Equal(t, "dev-ns", obj.Namespace)
	})

	t.Run("namespace matches: object returned", func(t *testing.T) {
		m := newMockClient(t)
		m.OnGetSetPod().Return(nil)
		fc := makeFilterClient(m, sel, "prod-ns")
		obj := &corev1.Pod{}
		require.NoError(t, fc.Get(ctx, types.NamespacedName{Name: "my-pod", Namespace: "prod-ns"}, obj))
		assert.Equal(t, "prod-ns", obj.Namespace)
	})

	t.Run("namespace does not match: NotFound error returned", func(t *testing.T) {
		m := newMockClient(t)
		m.OnGetSetPod().Return(nil)
		fc := makeFilterClient(m, sel) // dev-ns not seeded
		err := fc.Get(ctx, types.NamespacedName{Name: "my-pod", Namespace: "dev-ns"}, &corev1.Pod{})
		require.True(t, apierrors.IsNotFound(err))
	})

	t.Run("operator namespace always passes filter", func(t *testing.T) {
		m := newMockClient(t)
		m.OnGetSetPod().Return(nil)
		fc := makeFilterClient(m, sel) // no extra namespaces seeded
		obj := &corev1.Pod{}
		require.NoError(t, fc.Get(ctx, types.NamespacedName{Name: "my-pod", Namespace: testOperatorNS}, obj))
		assert.Equal(t, testOperatorNS, obj.Namespace)
	})

	t.Run("empty namespace (cluster-scoped resource) always passes filter", func(t *testing.T) {
		m := newMockClient(t)
		m.OnGetSetPod().Return(nil)
		fc := makeFilterClient(m, sel) // no extra namespaces seeded
		obj := &corev1.Pod{}
		require.NoError(t, fc.Get(ctx, types.NamespacedName{Name: "cluster-res"}, obj))
		assert.Equal(t, "", obj.Namespace)
	})
}
