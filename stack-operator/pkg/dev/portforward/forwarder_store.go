package portforward

import (
	"context"
	"fmt"
	"sync"
)

// forwarderStore is a store for Forwarders that handles the forwarder lifecycle.
type forwarderStore struct {
	forwarders map[string]Forwarder
	sync.Mutex
}

// ForwarderFactory is a function that can produce forwarders
type ForwarderFactory interface {
	NewForwarder(network, addr string) (Forwarder, error)
}

// ForwarderFactoryFunc is an adapter from a function to a DialerForwarderFactory
type ForwarderFactoryFunc func(network, addr string) (Forwarder, error)

func (f ForwarderFactoryFunc) NewForwarder(network, addr string) (Forwarder, error) {
	return f(network, addr)
}

// NewForwarderStore creates a new initialized forwarderStore
func NewForwarderStore() *forwarderStore {
	return &forwarderStore{
		forwarders: make(map[string]Forwarder),
	}
}

// GetOrCreateForwarder returns a cached Forwarder if it exists, or a new one.
//
// The forwarder will be running when returned and automatically removed from the store when it stops running.
func (s *forwarderStore) GetOrCreateForwarder(network, addr string, factory ForwarderFactory) (Forwarder, error) {
	s.Lock()
	defer s.Unlock()

	key := netAddrToKey(network, addr)

	fwd, ok := s.forwarders[key]
	if ok {
		return fwd, nil
	}

	fwd, err := factory.NewForwarder(network, addr)
	if err != nil {
		return nil, err
	}
	s.forwarders[key] = fwd

	// start the podForwarder in a goroutine
	go func() {
		// remove the podForwarder from the map when done running
		defer func() {
			s.Lock()
			defer s.Unlock()

			delete(s.forwarders, key)
		}()
		// TODO: cancel this at some point to GC?
		if err := fwd.Run(context.TODO()); err != nil {
			log.Error(err, "Forwarder returned with an error")
		} else {
			log.Info("Forwarder returned without an error")
		}
	}()

	return fwd, nil
}

// netAddrToKey returns the map key to use for this network+address tuple
func netAddrToKey(network, addr string) string {
	return fmt.Sprintf("%s/%s", network, addr)
}
