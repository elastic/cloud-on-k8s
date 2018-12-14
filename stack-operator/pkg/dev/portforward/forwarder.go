package portforward

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

// forwardingFromRegex is the stdout output from kubectl port-forward that contains the locally bound port.
var forwardingFromRegex = regexp.MustCompile(`Forwarding from (?P<localHostPort>\S+) -> \S+\n?`)

// forwarder enables redirecting tcp connections through "kubectl port-forward"
//
// - "kubectl port-forward" will be run as a subprocess
// - only one subprocess will be spawned concurrently regardless of the number of dials
type forwarder struct {
	network, addr string

	sync.Mutex

	// initChan is used to wait for the port-forwarder to be set up before redirecting connections
	initChan chan struct{}
	// viaErr is set when there's an error during initialization
	viaErr error
	// viaAddr is the address that we use when redirecting connections
	viaAddr string

	// commandFactory is used to facilitate testing without spawning processes
	commandFactory commandFactory

	// dialerFunc is used to facilitate testing without making new connections
	dialerFunc dialerFunc
}

var _ Forwarder = &forwarder{}

// commandFactory is a factory for commands
type commandFactory func(ctx context.Context, name string, arg ...string) command

// defaultCommandFactory is the default factory used for commands outside of tests
var defaultCommandFactory commandFactory = func(ctx context.Context, name string, arg ...string) command {
	return exec.CommandContext(ctx, name, arg...)
}

// command is an interface that declares the parts of exec.Cmd we use and facilitates testing
type command interface {
	StdoutPipe() (io.ReadCloser, error)
	Start() error
	Wait() error
}

// dialerFunc is a factory for connections
type dialerFunc func(ctx context.Context, network, address string) (net.Conn, error)

// defaultDialerFunc is the default dialer function we use outside of tests
var defaultDialerFunc dialerFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}

// NewForwarder returns a new initialized forwarder
func NewForwarder(network, addr string) *forwarder {
	return &forwarder{
		network:  network,
		addr:     addr,
		initChan: make(chan struct{}),

		commandFactory: defaultCommandFactory,
		dialerFunc:     defaultDialerFunc,
	}
}

// DialContext connects to the forwarder address using the provided context.
func (f *forwarder) DialContext(ctx context.Context) (net.Conn, error) {
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

// Run starts a forwarding process and blocks until either the port forwarding process fails or the context is done.
func (f *forwarder) Run(ctx context.Context) error {
	log.Info("Running port-forwarder for", "addr", f.addr)

	forwardingAddrChan := make(chan string)
	attemptErrorChan := make(chan error)

	go func() {
		if err := f.runPortForwardProcess(ctx, forwardingAddrChan); err != nil {
			attemptErrorChan <- err
			close(attemptErrorChan)
		}
	}()

	// used as a safeguard to ensure we only close the init channel once
	initCloser := sync.Once{}

	for {
		select {
		case err := <-attemptErrorChan:
			// can probably come up with a better error to set here in the future.
			f.viaErr = errors.New("not currently forwarding")

			// wrap this in a sync.Once because it will panic if it happens more than once
			initCloser.Do(func() {
				close(f.initChan)
			})

			return err
		case <-ctx.Done():
			return nil
		case forwardingAddr := <-forwardingAddrChan:
			// this should only happen once according to the currently experience behavior.
			log.Info("Ready to redirect connections", "addr", f.addr, "via", forwardingAddr)

			// wrap this in a sync.Once because it will panic if it happens more than once
			initCloser.Do(func() {
				close(f.initChan)
			})

			f.viaAddr = forwardingAddr
		}
	}
}

// runPortForwardProcess does a single attempt at setting up port-forward.
// after starting, it does not return until the process exits or the context is cancelled.
// the out parameter will receive a string, which is the local address port-forward is bound to
func (f *forwarder) runPortForwardProcess(ctx context.Context, out chan<- string) error {
	// derive a new context so we can ensure the process is (attempted) killed before we return and that we return as
	// soon as the process exits.
	runCtx, runCtxCancel := context.WithCancel(ctx)
	defer runCtxCancel()

	addrType, err := parseAddrType(f.addr)
	if err != nil {
		return err
	}

	_, port, err := net.SplitHostPort(f.addr)
	if err != nil {
		return err
	}

	cmd := f.commandFactory(
		runCtx,
		"kubectl",
		"port-forward",
		// bind to localhost specifically
		"--address",
		"127.0.0.1",
		"--namespace",
		addrType.namespace,
		addrType.TypeName(),
		":"+port,
	)

	// prepare to capture stdout
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	// parse stdout when it becomes available
	go func() {
		reader := bufio.NewReader(stdout)
		for {
			if ctx.Err() != nil {
				return
			}

			line, err := reader.ReadString('\n')
			if err != nil {
				// read partial string, likely an EOF, which means we are closing, nothing more to parse
				return
			}

			log.Info("port-forward stdout:", "line", line)

			localAddress := findForwardedFromLocalAddress(line)
			if localAddress != "" {
				out <- localAddress
			}
		}
	}()

	if err := cmd.Start(); err != nil {
		return err
	}

	// ensure that runCtx is cancelled if the command completes early
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		runCtxCancel()
	}()

	// wait for the context to be cancelled, which indicates the command either finished early or were cancelled
	<-runCtx.Done()

	return waitErr
}

// addrType encapsulates information about an address to a Kubernetes resource
type addrType struct {
	type_, name, namespace string
}

// TypeName returns the resource "type/name" for this address.
func (a addrType) TypeName() string {
	return fmt.Sprintf("%s/%s", a.type_, a.name)
}

// parseAddrType parses a kubernetes DNS-based address to a structured format
// only "service" types with a FQDN is currently supported.
func parseAddrType(addr string) (addrType, error) {
	// services generally look like this (as FQDN): {name}.{namespace}.svc.cluster.local
	if strings.Contains(addr, ".svc.") {
		parts := strings.SplitN(addr, ".", 3)
		name := parts[0]
		namespace := parts[1]
		return addrType{type_: "service", name: name, namespace: namespace}, nil
	}

	return addrType{}, fmt.Errorf("unsupported type: %s", addr)
}

// findForwardedFromLocalAddress finds the local address from the "Forwarded from" stdout output of port-forward
func findForwardedFromLocalAddress(line string) string {
	submatches := forwardingFromRegex.FindStringSubmatch(line)
	names := forwardingFromRegex.SubexpNames()

	for i, submatch := range submatches {
		if names[i] != "localHostPort" {
			continue
		}

		host, _, err := net.SplitHostPort(submatch)
		if err != nil {
			// not a host:port tuple, safe to ignore
			continue
		}

		// we only support forwarding over ipv4, so anything else can be ignored
		hostIp := net.ParseIP(host)
		if hostIp.To4() == nil {
			continue
		}

		return submatch
	}

	return ""
}
