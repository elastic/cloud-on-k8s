// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package portforward

import (
	"context"
	"fmt"
	"sync"
)

// ForwarderStore is a store for Forwarders that handles the forwarder lifecycle.
type ForwarderStore struct {
	forwarders map[string]Forwarder
	sync.Mutex
}

// ForwarderFactory is a function that can produce forwarders
type ForwarderFactory func(ctx context.Context, network, addr string) (Forwarder, error)

// NewForwarderStore creates a new initialized forwarderStore
func NewForwarderStore() *ForwarderStore {
	return &ForwarderStore{
		forwarders: make(map[string]Forwarder),
	}
}

// GetOrCreateForwarder returns a cached Forwarder if it exists, or a new one.
//
// The forwarder will be running when returned and automatically removed from the store when it stops running.
func (s *ForwarderStore) GetOrCreateForwarder(network, addr string, factory ForwarderFactory) (Forwarder, error) {
	s.Lock()
	defer s.Unlock()

	key := netAddrToKey(network, addr)

	fwd, ok := s.forwarders[key]
	if ok {
		return fwd, nil
	}

	fwd, err := factory(context.Background(), network, addr)
	if err != nil {
		return nil, err
	}
	s.forwarders[key] = fwd

	// run the forwarder in a goroutine
	go func() {
		// remove the forwarder from the map when done running
		defer func() {
			s.Lock()
			defer s.Unlock()

			delete(s.forwarders, key)
		}()
		// TODO: cancel this at some point to GC?
		if err := fwd.Run(context.Background()); err != nil {
			log.Error(err, "Forwarder returned with an error", "addr", addr)
		} else {
			log.V(2).Info("Forwarder returned without an error", "addr", addr)
		}
	}()

	return fwd, nil
}

// netAddrToKey returns the map key to use for this network+address tuple
func netAddrToKey(network, addr string) string {
	return fmt.Sprintf("%s/%s", network, addr)
}
