package portforward

import (
	"context"
	"fmt"
	"net"
	"sync"

	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.KBLog.WithName("portforward")

// ForwardingDialer is a dialer that uses a forwarder to redirect connections when dialing
type ForwardingDialer struct {
	forwarders map[string]Forwarder
	sync.Mutex

	// forwarderFactory is used to inject a custom forwarder during testing.
	forwarderFactory ForwarderFactory
}

// ForwarderFactory is a function that can produce forwarders
type ForwarderFactory func(network, addr string) Forwarder

// defaultForwarderFactory is the default forwarder factory used outside of tests
var defaultForwarderFactory ForwarderFactory = func(network, addr string) Forwarder {
	return NewForwarder(network, addr)
}

// Forwarder is something that can forward connections
type Forwarder interface {
	// Run starts the forwarder and is a blocking function
	Run(ctx context.Context) error
	// DialContext creates a connection to the forwarded address
	DialContext(ctx context.Context) (net.Conn, error)
}

// NewForwardingDialer creates a new, initialized ForwardingDialer
func NewForwardingDialer() *ForwardingDialer {
	return &ForwardingDialer{
		forwarders:       make(map[string]Forwarder),
		forwarderFactory: defaultForwarderFactory,
	}
}

// DialContext uses a cached internal forwarder to redirect connections.
//
// There is no garbage collection involved, so the redirect and forwarder will live for the duration of
// the process.
func (d *ForwardingDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	fwd := d.getOrCreateForwarder(network, addr)

	return fwd.DialContext(ctx)
}

// getOrCreateForwarder gets the current or creates and inserts a new forwarder for the given address
func (d *ForwardingDialer) getOrCreateForwarder(network, addr string) Forwarder {
	d.Lock()
	defer d.Unlock()

	key := netAddrToKey(network, addr)

	fwd, ok := d.forwarders[key]
	if !ok {
		fwd = d.forwarderFactory(network, addr)
		d.forwarders[key] = fwd

		// start the forwarder in a goroutine
		go func() {
			// remove the forwarder from the map when done running
			defer func() {
				d.Lock()
				defer d.Unlock()

				delete(d.forwarders, key)
			}()
			// TODO: cancel this at some point to GC?
			if err := fwd.Run(context.TODO()); err != nil {
				log.Error(err, "Forwarder returned with an error")
			} else {
				log.Info("Forwarder returned without an error")
			}
		}()
	}

	return fwd
}

// netAddrToKey returns the map key to use for this network+address tuple
func netAddrToKey(network, addr string) string {
	return fmt.Sprintf("%s/%s", network, addr)
}
