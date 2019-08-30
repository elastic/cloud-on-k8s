// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package k8s

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultTimeout is a reasonable timeout to use with the Client.
const DefaultTimeout = 1 * time.Minute

// WrapClient returns a Client that performs requests within DefaultTimeout.
func WrapClient(client client.Client) Client {
	return &clientWrapper{
		crClient: client,
		timeout:  DefaultTimeout,
	}
}

// Client wraps a controller-runtime client to use a
// default context with a timeout if no context is passed.
type Client interface {
	// WithContext returns a client configured to use the provided context on
	// subsequent requests, instead of one created from the preconfigured timeout.
	WithContext(ctx context.Context) Client
	// WithTimeout returns a client with an overridden timeout value,
	// to be used when no explicit context is passed.
	WithTimeout(timeout time.Duration) Client

	// Get wraps a controller-runtime client.Get call with a context.
	Get(key client.ObjectKey, obj runtime.Object) error
	// List wraps a controller-runtime client.List call with a context.
	List(list runtime.Object, opts ...client.ListOption) error
	// Create wraps a controller-runtime client.Create call with a context.
	Create(obj runtime.Object, opts ...client.CreateOption) error
	// Delete wraps a controller-runtime client.Delete call with a context.
	Delete(obj runtime.Object, opts ...client.DeleteOption) error
	// Update wraps a controller-runtime client.Update call with a context.
	Update(obj runtime.Object, opts ...client.UpdateOption) error
	// Status wraps a controller-runtime client.Status call.
	Status() StatusWriter
	// Patch patches the given obj in the Kubernetes cluster. obj must be a
	// struct pointer so that obj can be updated with the content returned by the Server.
	Patch(obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error
	// DeleteAllOf deletes all objects of the given type matching the given options.
	DeleteAllOf(obj runtime.Object, opts ...client.DeleteAllOfOption) error
}

type clientWrapper struct {
	crClient client.Client
	timeout  time.Duration
	ctx      context.Context // nil if not provided
}

// WithContext returns a client configured to use the provided context on
// subsequent requests, instead of one created from the preconfigured timeout.
func (w *clientWrapper) WithContext(ctx context.Context) Client {
	return &clientWrapper{
		crClient: w.crClient,
		ctx:      ctx,
	}
}

// WithTimeout returns a client with an overridden timeout value,
// to be used when no explicit context is passed.
func (w *clientWrapper) WithTimeout(timeout time.Duration) Client {
	return &clientWrapper{
		crClient: w.crClient,
		timeout:  timeout,
	}
}

// callWithContext calls f with the user-provided context. If no context was
// provided, it uses the default one.
func (w *clientWrapper) callWithContext(f func(ctx context.Context) error) error {
	var ctx context.Context
	if w.ctx != nil {
		// use the provided context
		ctx = w.ctx
	} else {
		// no context provided, use the default one
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), w.timeout)
		defer cancel()
	}
	return f(ctx)
}

// Get wraps a controller-runtime client.Get call with a context.
func (w *clientWrapper) Get(key client.ObjectKey, obj runtime.Object) error {
	return w.callWithContext(func(ctx context.Context) error {
		return w.crClient.Get(ctx, key, obj)
	})
}

// List wraps a controller-runtime client.List call with a context.
func (w *clientWrapper) List(list runtime.Object, opts ...client.ListOption) error {
	return w.callWithContext(func(ctx context.Context) error {
		return w.crClient.List(ctx, list, opts...)
	})
}

// Create wraps a controller-runtime client.Create call with a context.
func (w *clientWrapper) Create(obj runtime.Object, opts ...client.CreateOption) error {
	return w.callWithContext(func(ctx context.Context) error {
		return w.crClient.Create(ctx, obj, opts...)
	})
}

// Update wraps a controller-runtime client.Update call with a context.
func (w *clientWrapper) Update(obj runtime.Object, opts ...client.UpdateOption) error {
	return w.callWithContext(func(ctx context.Context) error {
		return w.crClient.Update(ctx, obj, opts...)
	})
}

// Patch wraps a controller-runtime client.Patch call with a context.
func (w *clientWrapper) Patch(obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	return w.callWithContext(func(ctx context.Context) error {
		return w.crClient.Patch(ctx, obj, patch, opts...)
	})
}

// Delete wraps a controller-runtime client.Delete call with a context.
func (w *clientWrapper) Delete(obj runtime.Object, opts ...client.DeleteOption) error {
	return w.callWithContext(func(ctx context.Context) error {
		return w.crClient.Delete(ctx, obj, opts...)
	})
}

// DeleteAllOf wraps a controller-runtime client.DeleteAllOf call with a context.
func (w *clientWrapper) DeleteAllOf(obj runtime.Object, opts ...client.DeleteAllOfOption) error {
	return w.callWithContext(func(ctx context.Context) error {
		return w.crClient.DeleteAllOf(ctx, obj, opts...)
	})
}

// StatusWriter wraps a client.StatusWrapper with a context.
type StatusWriter struct {
	client.StatusWriter
	w *clientWrapper
}

// Status wraps a controller-runtime client.Status call.
func (w *clientWrapper) Status() StatusWriter {
	return StatusWriter{
		StatusWriter: w.crClient.Status(),
		w:            w,
	}
}

// Update wraps a controller-runtime client.Status().Update call with a context.
func (s StatusWriter) Update(obj runtime.Object) error {
	return s.w.callWithContext(func(ctx context.Context) error {
		return s.StatusWriter.Update(ctx, obj)
	})
}

// Update wraps a controller-runtime client.Status().Update call with a context.
func (s StatusWriter) Patch(obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	return s.w.callWithContext(func(ctx context.Context) error {
		return s.StatusWriter.Patch(ctx, obj, patch, opts...)
	})
}
