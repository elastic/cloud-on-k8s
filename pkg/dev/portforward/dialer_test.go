// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package portforward

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type stubForwarder struct {
	network, addr string
	onRun         func(ctx context.Context) error
	onDialContext func(ctx context.Context) (net.Conn, error)
}

func (f *stubForwarder) Run(ctx context.Context) error {
	if f.onRun != nil {
		return f.onRun(ctx)
	}
	<-ctx.Done()
	return nil
}

func (f *stubForwarder) DialContext(ctx context.Context) (net.Conn, error) {
	if f.onDialContext != nil {
		return f.onDialContext(ctx)
	}
	return nil, nil
}

func TestForwardingDialer_DialContext(t *testing.T) {
	customError := errors.New("DialContext test error")

	d := NewForwardingDialer()
	d.forwarderFactory = func(_ context.Context, _ client.Client, network, addr string) (Forwarder, error) {
		return &stubForwarder{
			network: network, addr: addr,
			onDialContext: func(ctx context.Context) (net.Conn, error) {
				return nil, customError
			},
		}, nil
	}
	d.initOnce.Do(func() {}) // don't init with kubeconfig

	_, err := d.DialContext(context.Background(), "tcp", "localhost:8080")
	assert.Equal(t, customError, err)
}
