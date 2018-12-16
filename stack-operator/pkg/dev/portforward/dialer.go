package portforward

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	AutoPortForwardFlagName = "auto-port-forward"
)

var (
	AutoPortForwardFlag = false
	AutoDialer          = NewForwardingDialer()

	log = logf.KBLog.WithName("portforward")
)

// ForwardingDialer is a dialer that uses a podForwarder to redirect connections when dialing
type ForwardingDialer struct {
	forwarders map[string]Forwarder
	sync.Mutex

	initOnce sync.Once
	client   client.Client

	// forwarderFactory is used to inject a custom podForwarder during testing.
	forwarderFactory ForwarderFactory
}

// ForwarderFactory is a function that can produce forwarders
type ForwarderFactory interface {
	NewForwarder(client client.Client, network, addr string) (Forwarder, error)
}

// ForwarderFactoryFunc is a converter from a function to a ForwarderFactory
type ForwarderFactoryFunc func(client client.Client, network, addr string) (Forwarder, error)

func (f ForwarderFactoryFunc) NewForwarder(client client.Client, network, addr string) (Forwarder, error) {
	return f(client, network, addr)
}

// defaultForwarderFactory is the default podForwarder factory used outside of tests
var defaultForwarderFactory = func(client client.Client, network, addr string) (Forwarder, error) {
	if strings.Contains(addr, ".svc.cluster.local:") {
		// it looks like a service url, so forward as a service
		return NewServiceForwarder(client, network, addr)
	}
	return NewPodForwarder(network, addr)
}

// Forwarder is something that can forward connections
type Forwarder interface {
	// Run starts the podForwarder and is a blocking function
	Run(ctx context.Context) error
	// DialContext creates a connection to the forwarded address
	DialContext(ctx context.Context) (net.Conn, error)
}

// NewForwardingDialer creates a new, initialized ForwardingDialer
func NewForwardingDialer() *ForwardingDialer {
	return &ForwardingDialer{
		forwarders:       make(map[string]Forwarder),
		forwarderFactory: ForwarderFactoryFunc(defaultForwarderFactory),
	}
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

	fwd, err := d.getOrCreateForwarder(network, addr)
	if err != nil {
		return nil, err
	}

	return fwd.DialContext(ctx)
}

// getOrCreateForwarder gets the current or creates and inserts a new Forwarder for the given address
func (d *ForwardingDialer) getOrCreateForwarder(network, addr string) (Forwarder, error) {
	d.Lock()
	defer d.Unlock()

	key := netAddrToKey(network, addr)

	fwd, ok := d.forwarders[key]
	if ok {
		return fwd, nil
	}

	fwd, err := d.forwarderFactory.NewForwarder(d.client, network, addr)
	if err != nil {
		return nil, err
	}
	d.forwarders[key] = fwd

	// start the podForwarder in a goroutine
	go func() {
		// remove the podForwarder from the map when done running
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

	return fwd, nil
}

// netAddrToKey returns the map key to use for this network+address tuple
func netAddrToKey(network, addr string) string {
	return fmt.Sprintf("%s/%s", network, addr)
}
