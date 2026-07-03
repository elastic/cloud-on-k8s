// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nsmatch

import (
	"context"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// Currently the only filtering axis is namespace; the design can be extended to
// support additional predicate types if needed in the future (and in that case it can be exported to a different package).

// FilterClient wraps a cache-backed client.Client and filters List results by
// namespace using a NamespaceFlipNotifier. It delegates all other operations to
// the underlying client unchanged.
type FilterClient struct {
	client.Client
	nfn *NamespaceMatcher
}

// NewFilterClient returns a WrappedClient backed by the provided delegate.
func NewFilterClient(delegate client.Client, nfn *NamespaceMatcher) *FilterClient {
	return &FilterClient{Client: delegate, nfn: nfn}
}

// Get overrides the delegate's Get to apply namespace-selector filtering. When the
// requested key's namespace does not match, a synthetic NotFound error is returned
// without querying the delegate, so the caller treats the object as invisible,
// consistent with how List filters it out. As with a NotFound returned by the API
// server, obj is left untouched: callers must not rely on it being populated when
// an error is returned. Cluster-scoped objects have an empty key.Namespace, which
// always matches.
func (w *FilterClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if w.nfn.SelectorEnabled() && !w.nfn.Matches(key.Namespace) {
		return apierrors.NewNotFound(w.groupResource(obj), key.Name)
	}
	return w.Client.Get(ctx, key, obj, opts...)
}

// groupResource best-effort resolves the GroupResource of obj so that the synthetic
// NotFound error renders like one returned by the API server. Falls back to an empty
// (or group-only) GroupResource when the type is not registered in the scheme or the
// RESTMapper; apierrors.IsNotFound is unaffected either way.
func (w *FilterClient) groupResource(obj client.Object) schema.GroupResource {
	gvk, err := apiutil.GVKForObject(obj, w.Scheme())
	if err != nil {
		return schema.GroupResource{}
	}
	mapping, err := w.RESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupResource{Group: gvk.Group}
	}
	return mapping.Resource.GroupResource()
}

// List overrides the delegate's List to apply namespace-selector filtering on results.
func (w *FilterClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if err := w.Client.List(ctx, list, opts...); err != nil {
		return err
	}
	if !w.nfn.SelectorEnabled() {
		return nil
	}
	// Fast path for namespace-scoped lists: one Matches call decides the whole result.
	listOpts := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOpts)
	}
	if listOpts.Namespace != "" {
		if !w.nfn.Matches(listOpts.Namespace) {
			return apimeta.SetList(list, nil)
		}
		return nil
	}
	return filterByNamespace(list, w.nfn.Matches)
}

// filterByNamespace removes items from list whose namespace does not satisfy matches.
// Items are kept (fail-open) if their namespace cannot be determined, since an Accessor
// error is not expected for real list items and dropping objects on such an error would
// be surprising.
func filterByNamespace(list client.ObjectList, matches func(string) bool) error {
	items, err := apimeta.ExtractList(list)
	if err != nil {
		return err
	}
	items = slices.DeleteFunc(items, func(item runtime.Object) bool {
		acc, err := apimeta.Accessor(item)
		return err == nil && !matches(acc.GetNamespace())
	})
	return apimeta.SetList(list, items)
}
