// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package nsmatch

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Currently the only filtering axis is namespace; the design can be extended to
// support additional predicate types if needed in the future (and in that case it can be exported to a different package).

// FilterClient wraps a cache-backed client.Client and filters List results by
// namespace using a NamespaceFlipNotifier. It delegates all other operations to
// the underlying client unchanged.
type FilterClient struct {
	client.Client
	nfn *NamespaceFlipNotifier
}

// NewFilterClient returns a WrappedClient backed by the provided delegate.
func NewFilterClient(delegate client.Client, nfn *NamespaceFlipNotifier) *FilterClient {
	return &FilterClient{Client: delegate, nfn: nfn}
}

// Get overrides the delegate's Get to apply namespace-selector filtering. When the
// retrieved object's namespace does not match, a NotFound error is returned so the
// caller treats the object as invisible, consistent with how List filters it out.
func (w *FilterClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if err := w.Client.Get(ctx, key, obj, opts...); err != nil {
		return err
	}
	if !w.nfn.SelectorEnabled() {
		return nil
	}
	if !w.nfn.Matches(obj.GetNamespace()) {
		return apierrors.NewNotFound(schema.GroupResource{}, key.Name)
	}
	return nil
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

// filterByNamespace removes items from list whose namespace does not satisfy matches,
// reusing the extracted slice's backing array to avoid an extra allocation.
func filterByNamespace(list client.ObjectList, matches func(string) bool) error {
	items, err := apimeta.ExtractList(list)
	if err != nil {
		return err
	}
	filtered := items[:0]
	for _, item := range items {
		acc, err := apimeta.Accessor(item)
		if err != nil || matches(acc.GetNamespace()) {
			filtered = append(filtered, item)
		}
	}
	return apimeta.SetList(list, filtered)
}
