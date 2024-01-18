// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package k8s

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// NewFakeClient creates a new fake Kubernetes client.
func NewFakeClient(initObjs ...client.Object) Client {
	return fake.NewClientBuilder().
		WithObjects(initObjs...).
		WithStatusSubresource(initObjs...).
		WithScheme(clientgoscheme.Scheme).
		Build()
}

var (
	_ Client              = failingClient{}
	_ client.StatusWriter = failingSubClient{}
)

type failingSubClient struct {
	err error
}

func (fc failingSubClient) Create(_ context.Context, _ client.Object, _ client.Object, _ ...client.SubResourceCreateOption) error {
	return fc.err
}

func (fc failingSubClient) Get(_ context.Context, _ client.Object, _ client.Object, _ ...client.SubResourceGetOption) error {
	return fc.err
}

func (fc failingSubClient) Update(_ context.Context, _ client.Object, _ ...client.SubResourceUpdateOption) error {
	return fc.err
}

func (fc failingSubClient) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.SubResourcePatchOption) error {
	return fc.err
}

func (fc failingSubClient) GroupVersionKindFor(_ runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, fc.err
}

func (fc failingSubClient) IsObjectNamespaced(_ runtime.Object) (bool, error) {
	return false, fc.err
}

type failingClient struct {
	failingSubClient
	err error
}

// NewFailingClient returns a client that always returns the provided error when called.
func NewFailingClient(err error) Client {
	return failingClient{err: err}
}

func (fc failingClient) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return fc.err
}

func (fc failingClient) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return fc.err
}

func (fc failingClient) Create(_ context.Context, _ client.Object, _ ...client.CreateOption) error {
	return fc.err
}

func (fc failingClient) Delete(_ context.Context, _ client.Object, _ ...client.DeleteOption) error {
	return fc.err
}

func (fc failingClient) Update(_ context.Context, _ client.Object, _ ...client.UpdateOption) error {
	return fc.err
}

func (fc failingClient) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
	return fc.err
}

func (fc failingClient) DeleteAllOf(_ context.Context, _ client.Object, _ ...client.DeleteAllOfOption) error {
	return fc.err
}

func (fc failingClient) Status() client.StatusWriter {
	return fc.failingSubClient
}

func (fc failingClient) SubResource(_ string) client.SubResourceClient {
	return fc.failingSubClient
}

func (fc failingClient) Scheme() *runtime.Scheme {
	return Scheme()
}

func (fc failingClient) RESTMapper() meta.RESTMapper {
	return nil
}
