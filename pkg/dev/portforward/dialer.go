// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package portforward

import (
	"context"
	"net"
	"strings"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// ForwardingDialer is a dialer that uses a podForwarder to redirect connections when dialing
type ForwardingDialer struct {
	store *ForwarderStore

	initOnce sync.Once
	client   client.Client

	// forwarderFactory is used to inject a custom Forwarder during testing.
	forwarderFactory ForwardingDialerForwarderFactory
}

// ForwardingDialerForwarderFactory is a function that can produce forwarders
type ForwardingDialerForwarderFactory func(ctx context.Context, client client.Client, network, addr string) (Forwarder, error)

// NewForwardingDialer creates a new, initialized ForwardingDialer
func NewForwardingDialer() *ForwardingDialer {
	return &ForwardingDialer{
		store:            NewForwarderStore(),
		forwarderFactory: defaultForwarderFactory,
	}
}

// defaultForwarderFactory is the default podForwarder factory used outside of tests
var defaultForwarderFactory = ForwardingDialerForwarderFactory(
	func(ctx context.Context, client client.Client, network, addr string) (Forwarder, error) {
		if strings.Contains(addr, ".svc:") || strings.Contains(addr, ".svc.") {
			// it looks like a service url, so forward as a service
			return NewServiceForwarder(client, network, addr)
		}
		clientset, err := newDefaultKubernetesClientset()
		if err != nil {
			return nil, err
		}
		return NewPodForwarder(ctx, network, addr, clientset)
	},
)

// Forwarder is something that can forward connections
type Forwarder interface {
	// Run starts the podForwarder and is a blocking function
	Run(ctx context.Context) error
	// DialContext creates a connection to the forwarded address
	DialContext(ctx context.Context) (net.Conn, error)
}

// initIfRequired initializes the dialer once if required.
func (d *ForwardingDialer) initIfRequired() {
	d.initOnce.Do(func() {
		restConfig, err := config.GetConfig()
		if err != nil {
			panic(err)
		}

		client, err := client.New(restConfig, client.Options{})
		if err != nil {
			panic(err)
		}

		d.client = client
	})
}

// DialContext uses a cached internal podForwarder to redirect connections.
//
// There is no garbage collection involved, so the redirect and podForwarder will live for the duration of
// the process.
func (d *ForwardingDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	d.initIfRequired()

	fwd, err := d.store.GetOrCreateForwarder(network, addr, d.newForwarder)
	if err != nil {
		return nil, err
	}

	return fwd.DialContext(ctx)
}

// newForwarder adapts our internal forwarder factory to the forwarderStore one.
func (d *ForwardingDialer) newForwarder(ctx context.Context, network, addr string) (Forwarder, error) {
	return d.forwarderFactory(ctx, d.client, network, addr)
}
