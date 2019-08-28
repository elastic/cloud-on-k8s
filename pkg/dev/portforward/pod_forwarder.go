// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package portforward

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"

	"github.com/elastic/cloud-on-k8s/pkg/utils/k8s"
	utilsnet "github.com/elastic/cloud-on-k8s/pkg/utils/net"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// PodForwarder enables redirecting tcp connections through "kubectl port-forward" tooling
type PodForwarder struct {
	network, addr string
	podNSN        types.NamespacedName

	// clientset is used to stop the pod forwarder if the pod is deleted, may be set to nil to skip checking
	clientset *kubernetes.Clientset

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

var _ Forwarder = &PodForwarder{}

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
func NewPodForwarder(network, addr string, clientset *kubernetes.Clientset) (*PodForwarder, error) {
	podNSN, err := parsePodAddr(addr, clientset)
	if err != nil {
		return nil, err
	}

	return &PodForwarder{
		network: network,
		addr:    addr,

		podNSN:    *podNSN,
		clientset: clientset,

		initChan: make(chan struct{}),

		ephemeralPortFinder:  utilsnet.GetRandomPort,
		portForwarderFactory: defaultPortForwarderFactory,
		dialerFunc:           defaultDialerFunc,
	}, nil
}

// newDefaultKubernetesClientset creates a new Clientset
func newDefaultKubernetesClientset() (*kubernetes.Clientset, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

// podDNSRegex matches pods FQDN such as {name}.{namespace}.pod
var podDNSRegex = regexp.MustCompile(`^.+\..+$`)

// podIPRegex matches any ipv4 address.
var podIPv4Regex = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)

// parsePodAddr parses the pod name and namespace from an address.
func parsePodAddr(addr string, clientSet *kubernetes.Clientset) (*types.NamespacedName, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if podIPv4Regex.MatchString(host) {
		// we got an IP address
		// try to map it to a pod name and namespace
		return getPodWithIP(host, clientSet)
	}
	if podDNSRegex.MatchString(host) {
		// retrieve pod name and namespace from addr
		parts := strings.SplitN(host, ".", 4)
		if len(parts) <= 1 {
			return nil, fmt.Errorf("unsupported pod address format: %s", host)
		}
		if len(parts) == 2 || parts[2] == syntheticDNSSegment {
			// podname.ns[.pod] from service forwarder or direct call
			return &types.NamespacedName{Namespace: parts[1], Name: parts[0]}, nil
		}
		// podname.subdomain.ns
		return &types.NamespacedName{Namespace: parts[2], Name: parts[0]}, nil

	}
	return nil, fmt.Errorf("unsupported pod address format: %s", host)
}

// getPodWithIP requests the apiserver for pods with the given IP assigned.
func getPodWithIP(ip string, clientSet *kubernetes.Clientset) (*types.NamespacedName, error) {
	pods, err := clientSet.CoreV1().
		Pods("").
		List(metav1.ListOptions{
			FieldSelector: fmt.Sprintf("status.podIP=%s", ip),
		})
	if err != nil {
		return nil, err
	}
	if pods == nil || len(pods.Items) == 0 {
		return nil, fmt.Errorf("pod with IP %s not found", ip)
	}
	nsn := k8s.ExtractNamespacedName(&(pods.Items[0].ObjectMeta))
	return &nsn, nil
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
func (f *PodForwarder) DialContext(ctx context.Context) (net.Conn, error) {
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

	log.V(1).Info("Redirecting dial call", "addr", f.addr, "via", f.viaAddr)
	return f.dialerFunc(ctx, f.network, f.viaAddr)
}

// Run starts a port forwarder and blocks until either the port forwarding fails or the context is done.
func (f *PodForwarder) Run(ctx context.Context) error {
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

	if f.clientset != nil {
		log.V(1).Info("Watching pod for changes", "namespace", f.podNSN.Namespace, "pod_name", f.podNSN.Name)
		w, err := f.clientset.CoreV1().Pods(f.podNSN.Namespace).Watch(metav1.ListOptions{
			FieldSelector: fields.OneTermEqualSelector("metadata.name", f.podNSN.Name).String(),
		})
		if err != nil {
			return fmt.Errorf("unable to watch pod %s for changes: %s", f.podNSN, err)
		}
		defer w.Stop()

		go func() {
			for {
				select {
				case evt := <-w.ResultChan():
					if evt.Type == watch.Deleted || evt.Type == watch.Error || evt.Type == "" {
						log.V(1).Info(
							"Pod is deleted or watch failed/closed, closing pod forwarder",
							"namespace", f.podNSN.Namespace,
							"pod_name", f.podNSN.Name,
						)
						runCtxCancel()
						return
					}
				case <-runCtx.Done():
					return
				}
			}
		}()
	}

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
