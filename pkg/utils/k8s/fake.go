// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

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
	_ Client              = failingClient{}
	_ client.StatusWriter = failingStatusWriter{}
)

type failingClient struct {
	err error
}

// NewFailingClient returns a client that always returns the provided error when called.
func NewFailingClient(err error) Client {
	return failingClient{err: err}
}

func (fc failingClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return fc.err
}

func (fc failingClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return fc.err
}

func (fc failingClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return fc.err
}

func (fc failingClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return fc.err
}

func (fc failingClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return fc.err
}

func (fc failingClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return fc.err
}

func (fc failingClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return fc.err
}

func (fc failingClient) Status() client.StatusWriter {
	return failingStatusWriter{err: fc.err} //nolint:gosimple
}

func (fc failingClient) Scheme() *runtime.Scheme {
	return Scheme()
}

func (fc failingClient) RESTMapper() meta.RESTMapper {
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
