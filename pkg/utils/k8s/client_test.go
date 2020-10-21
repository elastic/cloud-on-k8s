// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package k8s

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ctxKey struct{}

var (
	userProvidedContextKey      = ctxKey{}
	errUsingUserProvidedContext = errors.New("using user-provided context")
)

func TestClient(t *testing.T) {
	tests := []struct {
		name string
		call func(c Client) error
	}{
		{
			name: "Get",
			call: func(c Client) error {
				return c.Get(types.NamespacedName{}, nil)
			},
		},
		{
			name: "List",
			call: func(c Client) error {
				return c.List(nil, nil)
			},
		},
		{
			name: "Create",
			call: func(c Client) error {
				return c.Create(nil)
			},
		},
		{
			name: "Update",
			call: func(c Client) error {
				return c.Update(nil)
			},
		},
		{
			name: "Status update",
			call: func(c Client) error {
				return c.Status().Update(nil)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := WrapClient(mockedClient{})

			// default behaviour should just return a nil error
			require.Nil(t, tt.call(c))

			// pass a custom context with the call
			ctx := context.WithValue(context.Background(), userProvidedContextKey, userProvidedContextKey)
			require.Equal(t, errUsingUserProvidedContext, tt.call(c.WithContext(ctx)))
		})
	}
}

// mockedClient's only purpose is to perform checks against the context
// passed in from the surrounding Client.
type mockedClient struct{}

func (m mockedClient) checkCtx(ctx context.Context) error {
	if ctx == nil {
		return errors.New("using no context")
	}
	if ctx.Value(userProvidedContextKey) == userProvidedContextKey {
		return errUsingUserProvidedContext
	}

	return nil
}

func (m mockedClient) Get(ctx context.Context, key client.ObjectKey, obj runtime.Object) error {
	return m.checkCtx(ctx)
}

func (m mockedClient) List(ctx context.Context, list runtime.Object, opts ...client.ListOption) error {
	return m.checkCtx(ctx)
}

func (m mockedClient) Create(ctx context.Context, obj runtime.Object, opts ...client.CreateOption) error {
	return m.checkCtx(ctx)
}

func (m mockedClient) Delete(ctx context.Context, obj runtime.Object, opts ...client.DeleteOption) error {
	return m.checkCtx(ctx)
}

func (m mockedClient) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	return m.checkCtx(ctx)
}

func (m mockedClient) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	return m.checkCtx(ctx)
}

func (m mockedClient) Status() client.StatusWriter {
	return mockedStatusWriter{c: m}
}

func (m mockedClient) DeleteAllOf(ctx context.Context, obj runtime.Object, opts ...client.DeleteAllOfOption) error {
	return m.checkCtx(ctx)
}

type mockedStatusWriter struct {
	c mockedClient
}

func (m mockedStatusWriter) Update(ctx context.Context, obj runtime.Object, opts ...client.UpdateOption) error {
	return m.c.checkCtx(ctx)
}

func (m mockedStatusWriter) Patch(ctx context.Context, obj runtime.Object, patch client.Patch, opts ...client.PatchOption) error {
	return m.c.checkCtx(ctx)
}
