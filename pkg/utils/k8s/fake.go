// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package k8s

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// NewFakeClient creates a new fake Kubernetes client.
func NewFakeClient(initObjs ...runtime.Object) Client {
	return fake.NewClientBuilder().WithRuntimeObjects(initObjs...).WithScheme(clientgoscheme.Scheme).Build()
}

var (
	_ Client              = &FailingClient{}
	_ client.StatusWriter = failingStatusWriter{}
)

// FailingClient is a k8s client that fails with a given error by default.  When erroring
// is false by calling 'DisableFailing', then underlying client is called.
type FailingClient struct {
	client   Client
	erroring bool
	err      error
}

// NewFailingClient returns a client that returns the provided error when called and 'DisableFailing' has not been called.
// After 'DisableFailing' is called, the client calls the underlying k8s client.
func NewFailingClient(err error, initObjs ...runtime.Object) Client {
	return &FailingClient{
		client:   fake.NewClientBuilder().WithRuntimeObjects(initObjs...).WithScheme(clientgoscheme.Scheme).Build(),
		erroring: true,
		err:      err,
	}
}

// Get satisfies the controller-runtime client interface.
func (fc *FailingClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	if !fc.erroring {
		return fc.client.Get(ctx, key, obj)
	}
	return fc.err
}

// List satisfies the controller-runtime client interface.
func (fc *FailingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if !fc.erroring {
		return fc.client.List(ctx, list, opts...)
	}
	return fc.err
}

// Create satisfies the controller-runtime client interface.
func (fc *FailingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if !fc.erroring {
		return fc.client.Create(ctx, obj, opts...)
	}
	return fc.err
}

// Delete satisfies the controller-runtime client interface.
func (fc *FailingClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if !fc.erroring {
		return fc.client.Delete(ctx, obj, opts...)
	}
	return fc.err
}

// Update satisfies the controller-runtime client interface.
func (fc *FailingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if !fc.erroring {
		return fc.client.Update(ctx, obj, opts...)
	}
	return fc.err
}

// Patch satisfies the controller-runtime client interface.
func (fc *FailingClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if !fc.erroring {
		return fc.client.Patch(ctx, obj, patch, opts...)
	}
	return fc.err
}

// DeleteAllOf satisfies the controller-runtime client interface.
func (fc *FailingClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	if !fc.erroring {
		return fc.client.DeleteAllOf(ctx, obj, opts...)
	}
	return fc.err
}

// DisableFailing will stop the client from failing, and will call the underyling k8s client.
func (fc *FailingClient) DisableFailing() {
	fc.erroring = false
}

// EnableFailing will cause the client to begin failing.
func (fc *FailingClient) EnableFailing() {
	fc.erroring = true
}

func (fc *FailingClient) Status() client.StatusWriter {
	return failingStatusWriter{err: fc.err}
}

func (fc *FailingClient) Scheme() *runtime.Scheme {
	return Scheme()
}

func (fc *FailingClient) RESTMapper() meta.RESTMapper {
	return nil
}

type failingStatusWriter struct {
	err error
}

func (fsw failingStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return fsw.err
}

func (fsw failingStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return fsw.err
}
