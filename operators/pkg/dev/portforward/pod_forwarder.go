package portforward

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

// podForwarder enables redirecting tcp connections through "kubectl port-forward" tooling
type podForwarder struct {
	network, addr string
	podNSN        types.NamespacedName

	// initChan is used to wait for the port-forwarder to be set up before redirecting connections
	initChan chan struct{}
	// viaErr is set when there's an error during initialization
	viaErr error
	// viaAddr is the address that we use when redirecting connections
	viaAddr string

	// ephemeralPortFinder is used to find an available ephemeral port
	ephemeralPortFinder func() (string, error)

	// portForwarderFactory is used to facilitate testing without using the API
	portForwarderFactory PortForwarderFactory

	// dialerFunc is used to facilitate testing without making new connections
	dialerFunc dialerFunc
}

var _ Forwarder = &podForwarder{}

// PortForwarderFactory is a factory for port forwarders
type PortForwarderFactory func(
	ctx context.Context,
	namespace, podName string,
	ports []string,
	readyChan chan struct{},
) (PortForwarder, error)

// PortForwarder is a port forwarder that may be started.
type PortForwarder interface {
	ForwardPorts() error
}

// dialerFunc is a factory for connections
type dialerFunc func(ctx context.Context, network, address string) (net.Conn, error)

// NewPodForwarder returns a new initialized podForwarder
func NewPodForwarder(network, addr string) (*podForwarder, error) {
	podNSN, err := parsePodAddr(addr)
	if err != nil {
		return nil, err
	}

	return &podForwarder{
		network: network,
		addr:    addr,

		podNSN: *podNSN,

		initChan: make(chan struct{}),

		ephemeralPortFinder:  defaultEphemeralPortFinder,
		portForwarderFactory: defaultPortForwarderFactory,
		dialerFunc:           defaultDialerFunc,
	}, nil
}

// parsePodAddr parses the pod name and namespace from an address
func parsePodAddr(addr string) (*types.NamespacedName, error) {
	// (our) pods generally look like this (as FQDN): {name}.{namespace}.pod.cluster.local
	// TODO: subdomains in pod names would change this.
	parts := strings.SplitN(addr, ".", 3)

	if len(parts) <= 2 {
		return nil, fmt.Errorf("unsupported pod address format: %s", addr)
	}

	return &types.NamespacedName{Namespace: parts[1], Name: parts[0]}, nil
}

// defaultEphemeralPortFinder finds an ephemeral port by binding to :0 and checking what port was bound
var defaultEphemeralPortFinder = func() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	addr := listener.Addr().String()

	if err := listener.Close(); err != nil {
		return "", err
	}

	_, localPort, err := net.SplitHostPort(addr)

	return localPort, err
}

// defaultPortForwarderFactory is the default factory used for port forwarders outside of tests
var defaultPortForwarderFactory PortForwarderFactory = func(
	ctx context.Context,
	namespace, podName string,
	ports []string,
	readyChan chan struct{},
) (PortForwarder, error) {
	return newKubectlPortForwarder(ctx, namespace, podName, ports, readyChan)
}

// defaultDialerFunc is the default dialer function we use outside of tests
var defaultDialerFunc dialerFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}

// DialContext connects to the podForwarder address using the provided context.
func (f *podForwarder) DialContext(ctx context.Context) (net.Conn, error) {
	// wait until we're initialized or context is done
	select {
	case <-f.initChan:
	case <-ctx.Done():
	}

	// context has an error, so we can give up, most likely exceeded our timeout
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// we have an error to return
	if f.viaErr != nil {
		return nil, f.viaErr
	}

	log.Info("Redirecting dial call", "addr", f.addr, "via", f.viaAddr)
	return f.dialerFunc(ctx, f.network, f.viaAddr)
}

// Run starts a port forwarder and blocks until either the port forwarding fails or the context is done.
func (f *podForwarder) Run(ctx context.Context) error {
	log.Info("Running port-forwarder for", "addr", f.addr)
	defer log.Info("No longer running port-forwarder for", "addr", f.addr)

	// used as a safeguard to ensure we only close the init channel once
	initCloser := sync.Once{}

	// wrap this in a sync.Once because it will panic if it happens more than once
	// ensure that initChan is closed even if we were never ready.
	defer initCloser.Do(func() {
		close(f.initChan)
	})

	// derive a new context so we can ensure the port-forwarding is stopped before we return and that we return as
	// soon as the port-forwarding stops, whichever occurs first
	runCtx, runCtxCancel := context.WithCancel(ctx)
	defer runCtxCancel()

	_, port, err := net.SplitHostPort(f.addr)
	if err != nil {
		return err
	}

	// find an available local ephemeral port
	localPort, err := f.ephemeralPortFinder()
	if err != nil {
		return err
	}

	readyChan := make(chan struct{})
	fwd, err := f.portForwarderFactory(
		runCtx,
		f.podNSN.Namespace,
		f.podNSN.Name,
		[]string{localPort + ":" + port},
		readyChan,
	)
	if err != nil {
		return err
	}

	// wait for our context to be done or the port forwarder to become ready
	go func() {
		select {
		case <-runCtx.Done():
		case <-readyChan:
			f.viaAddr = "127.0.0.1:" + localPort

			log.Info("Ready to redirect connections", "addr", f.addr, "via", f.viaAddr)

			// wrap this in a sync.Once because it will panic if it happens more than once, which it may if our
			// outer function returned just as readyChan was closed.
			initCloser.Do(func() {
				close(f.initChan)
			})
		}
	}()

	err = fwd.ForwardPorts()
	f.viaErr = errors.New("not currently forwarding")
	return err
}
