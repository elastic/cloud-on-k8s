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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	cachemock "github.com/elastic/cloud-on-k8s/v3/pkg/utils/test/mock"
)

const testOperatorNS = "elastic-system"

func mustSelector(t *testing.T, matchLabels map[string]string) labels.Selector {
	t.Helper()
	ls := metav1.LabelSelector{MatchLabels: matchLabels}
	sel, err := metav1.LabelSelectorAsSelector(&ls)
	if err != nil {
		t.Fatalf("LabelSelectorAsSelector error %s", err.Error())
	}
	return sel
}

// makeFilterClient builds a FilterClient whose matcher is backed by a mock cache:
// namespaces named in matchedNSes resolve to labels that satisfy sel, every other
// namespace resolves to no labels (and therefore does not match).
func makeFilterClient(t *testing.T, delegate client.Client, sel labels.Selector, matchedNSes ...string) *FilterClient {
	t.Helper()

	matched := make(map[string]struct{}, len(matchedNSes))
	for _, ns := range matchedNSes {
		matched[ns] = struct{}{}
	}

	mc := cachemock.NewCache(t)
	mc.On("Get", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			key, _ := args.Get(1).(client.ObjectKey)
			ns, _ := args.Get(2).(*corev1.Namespace)
			ns.ObjectMeta = metav1.ObjectMeta{Name: key.Name}
			if _, ok := matched[key.Name]; ok {
				ns.Labels = map[string]string{"env": "prod"}
			}
		}).
		Return(nil).
		Maybe()

	nfn := NewNamespaceMatcher(sel, testOperatorNS)
	nfn.SetCache(mc)
	return NewFilterClient(delegate, nfn)
}

func pod(name, ns string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
}

func podNames(list *corev1.PodList) []string {
	names := make([]string, len(list.Items))
	for i, p := range list.Items {
		names[i] = p.Name
	}
	return names
}

// fakeClientListErr returns a fake.Client whose List calls always fail with err.
func fakeClientListErr(err error) client.Client {
	return fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		List: func(_ context.Context, _ client.WithWatch, _ client.ObjectList, _ ...client.ListOption) error {
			return err
		},
	}).Build()
}

// fakeClientGetErr returns a fake.Client whose Get calls always fail with err.
func fakeClientGetErr(err error) client.Client {
	return fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
			return err
		},
	}).Build()
}

func TestFilterClientList(t *testing.T) {
	sel := mustSelector(t, map[string]string{"env": "prod"})

	t.Run("delegate error is propagated without filtering", func(t *testing.T) {
		fc := NewFilterClient(fakeClientListErr(errors.New("api server unavailable")), NewNamespaceMatcher(sel, testOperatorNS))
		require.Error(t, fc.List(t.Context(), &corev1.PodList{}))
	})

	t.Run("nil notifier: all items returned unfiltered", func(t *testing.T) {
		fc := NewFilterClient(fake.NewClientBuilder().WithObjects(pod("a", "ns-a"), pod("b", "ns-b")).Build(), nil)
		list := &corev1.PodList{}
		require.NoError(t, fc.List(t.Context(), list))
		assert.Len(t, list.Items, 2)
	})

	t.Run("selector disabled: all items returned unfiltered", func(t *testing.T) {
		fc := NewFilterClient(fake.NewClientBuilder().WithObjects(pod("a", "ns-a"), pod("b", "ns-b")).Build(), NewNamespaceMatcher(nil, testOperatorNS))
		list := &corev1.PodList{}
		require.NoError(t, fc.List(t.Context(), list))
		assert.Len(t, list.Items, 2)
	})

	t.Run("namespace-scoped list, namespace matches: items unchanged", func(t *testing.T) {
		fc := makeFilterClient(t, fake.NewClientBuilder().WithObjects(pod("a", "prod-ns"), pod("b", "prod-ns")).Build(), sel, "prod-ns")
		list := &corev1.PodList{}
		require.NoError(t, fc.List(t.Context(), list, client.InNamespace("prod-ns")))
		assert.Len(t, list.Items, 2)
	})

	t.Run("namespace-scoped list, namespace does not match: all items cleared", func(t *testing.T) {
		fc := makeFilterClient(t, fake.NewClientBuilder().WithObjects(pod("a", "dev-ns"), pod("b", "dev-ns")).Build(), sel) // dev-ns not seeded
		list := &corev1.PodList{}
		require.NoError(t, fc.List(t.Context(), list, client.InNamespace("dev-ns")))
		assert.Empty(t, list.Items)
	})

	t.Run("cluster-scoped list, all namespaces match: all items kept", func(t *testing.T) {
		fc := makeFilterClient(t, fake.NewClientBuilder().WithObjects(pod("a", "ns-1"), pod("b", "ns-2")).Build(), sel, "ns-1", "ns-2")
		list := &corev1.PodList{}
		require.NoError(t, fc.List(t.Context(), list))
		assert.ElementsMatch(t, []string{"a", "b"}, podNames(list))
	})

	t.Run("cluster-scoped list, no namespace matches: all items removed", func(t *testing.T) {
		fc := makeFilterClient(t, fake.NewClientBuilder().WithObjects(pod("a", "ns-1"), pod("b", "ns-2")).Build(), sel) // neither seeded
		list := &corev1.PodList{}
		require.NoError(t, fc.List(t.Context(), list))
		assert.Empty(t, list.Items)
	})

	t.Run("cluster-scoped list, mixed namespaces: only matching items kept", func(t *testing.T) {
		fc := makeFilterClient(t, fake.NewClientBuilder().WithObjects(pod("a", "prod-ns"), pod("b", "dev-ns"), pod("c", "prod-ns")).Build(), sel, "prod-ns")
		list := &corev1.PodList{}
		require.NoError(t, fc.List(t.Context(), list))
		assert.ElementsMatch(t, []string{"a", "c"}, podNames(list))
	})

	t.Run("cluster-scoped list, empty result: stays empty", func(t *testing.T) {
		fc := makeFilterClient(t, fake.NewClientBuilder().Build(), sel, "prod-ns")
		list := &corev1.PodList{}
		require.NoError(t, fc.List(t.Context(), list))
		assert.Empty(t, list.Items)
	})

	t.Run("operator namespace always passes filter", func(t *testing.T) {
		fc := makeFilterClient(t, fake.NewClientBuilder().WithObjects(pod("op", testOperatorNS), pod("b", "dev-ns")).Build(), sel) // no extra namespaces seeded
		list := &corev1.PodList{}
		require.NoError(t, fc.List(t.Context(), list))
		assert.ElementsMatch(t, []string{"op"}, podNames(list))
	})

	t.Run("empty namespace (cluster-scoped resource) always passes filter", func(t *testing.T) {
		fc := makeFilterClient(t, fake.NewClientBuilder().WithObjects(pod("cluster-res", ""), pod("b", "dev-ns")).Build(), sel) // no extra namespaces seeded
		list := &corev1.PodList{}
		require.NoError(t, fc.List(t.Context(), list))
		assert.ElementsMatch(t, []string{"cluster-res"}, podNames(list))
	})
}

func TestFilterClientGet(t *testing.T) {
	sel := mustSelector(t, map[string]string{"env": "prod"})

	t.Run("delegate error is propagated without filtering", func(t *testing.T) {
		fc := NewFilterClient(fakeClientGetErr(errors.New("api server unavailable")), NewNamespaceMatcher(sel, testOperatorNS))
		// testOperatorNS short-circuits the selector check, so the request reaches
		// the delegate without ever consulting the (unset) cache.
		require.Error(t, fc.Get(t.Context(), client.ObjectKey{Name: "my-pod", Namespace: testOperatorNS}, &corev1.Pod{}))
	})

	t.Run("nil notifier: object returned unfiltered", func(t *testing.T) {
		fc := NewFilterClient(fake.NewClientBuilder().WithObjects(pod("my-pod", "dev-ns")).Build(), nil)
		obj := &corev1.Pod{}
		require.NoError(t, fc.Get(t.Context(), types.NamespacedName{Name: "my-pod", Namespace: "dev-ns"}, obj))
		assert.Equal(t, "dev-ns", obj.Namespace)
	})

	t.Run("selector disabled: object returned unfiltered", func(t *testing.T) {
		fc := NewFilterClient(fake.NewClientBuilder().WithObjects(pod("my-pod", "dev-ns")).Build(), NewNamespaceMatcher(nil, testOperatorNS))
		obj := &corev1.Pod{}
		require.NoError(t, fc.Get(t.Context(), types.NamespacedName{Name: "my-pod", Namespace: "dev-ns"}, obj))
		assert.Equal(t, "dev-ns", obj.Namespace)
	})

	t.Run("namespace matches: object returned", func(t *testing.T) {
		fc := makeFilterClient(t, fake.NewClientBuilder().WithObjects(pod("my-pod", "prod-ns")).Build(), sel, "prod-ns")
		obj := &corev1.Pod{}
		require.NoError(t, fc.Get(t.Context(), types.NamespacedName{Name: "my-pod", Namespace: "prod-ns"}, obj))
		assert.Equal(t, "prod-ns", obj.Namespace)
	})

	t.Run("namespace does not match: NotFound error, object untouched, delegate not queried", func(t *testing.T) {
		delegateCalled := false
		delegate := fake.NewClientBuilder().WithObjects(pod("my-pod", "dev-ns")).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				delegateCalled = true
				return c.Get(ctx, key, obj, opts...)
			},
		}).Build()
		fc := makeFilterClient(t, delegate, sel) // dev-ns not seeded
		obj := &corev1.Pod{}
		err := fc.Get(t.Context(), types.NamespacedName{Name: "my-pod", Namespace: "dev-ns"}, obj)
		require.True(t, apierrors.IsNotFound(err))
		// The object must be left exactly as passed in, like a real API-server NotFound:
		// callers (and the data of the hidden resource) must not observe a populated object.
		assert.Empty(t, obj.Name)
		assert.Empty(t, obj.Namespace)
		assert.False(t, delegateCalled)
	})

	t.Run("operator namespace always passes filter", func(t *testing.T) {
		fc := makeFilterClient(t, fake.NewClientBuilder().WithObjects(pod("my-pod", testOperatorNS)).Build(), sel) // no extra namespaces seeded
		obj := &corev1.Pod{}
		require.NoError(t, fc.Get(t.Context(), types.NamespacedName{Name: "my-pod", Namespace: testOperatorNS}, obj))
		assert.Equal(t, testOperatorNS, obj.Namespace)
	})

	t.Run("empty namespace (cluster-scoped resource) always passes filter", func(t *testing.T) {
		fc := makeFilterClient(t, fake.NewClientBuilder().WithObjects(pod("cluster-res", "")).Build(), sel) // no extra namespaces seeded
		obj := &corev1.Pod{}
		require.NoError(t, fc.Get(t.Context(), types.NamespacedName{Name: "cluster-res"}, obj))
		assert.Equal(t, "", obj.Namespace)
	})
}
