package portforward

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// serviceForwarder forwards one port of a service
type serviceForwarder struct {
	network, addr          string
	serviceName, namespace string

	// client is used to look up the service and pods selected by the service during dialing
	client client.Client

	store *forwarderStore

	// podForwarderFactory enables injecting a custom forwarder factory in tests
	podForwarderFactory ForwarderFactory
}

var _ Forwarder = &serviceForwarder{}

// defaultPodForwarderFactory is the default pod forwarder factory used outside of tests
var defaultPodForwarderFactory = ForwarderFactoryFunc(func(network, addr string) (Forwarder, error) {
	return NewPodForwarder(network, addr)
})

// NewServiceForwarder returns a new initialized service forwarder
func NewServiceForwarder(client client.Client, network, addr string) (*serviceForwarder, error) {
	serviceName, namespace := parseServiceAddr(addr)

	return &serviceForwarder{
		network: network,
		addr:    addr,

		client: client,

		serviceName: serviceName,
		namespace:   namespace,

		store:               NewForwarderStore(),
		podForwarderFactory: defaultPodForwarderFactory,
	}, nil
}

// parseServiceAddr parses the service name and namespace from a connection address
func parseServiceAddr(addr string) (name string, namespace string) {
	// services generally look like this (as FQDN): {name}.{namespace}.svc.cluster.local
	parts := strings.SplitN(addr, ".", 3)
	name = parts[0]
	namespace = parts[1]
	return
}

// Run starts the service forwarder, blocking until it's done
func (f *serviceForwarder) Run(ctx context.Context) error {
	// TODO: /could/ consider snipping connections here when pods turn unready, but that does not match the default
	// Service behavior
	<-ctx.Done()
	return nil
}

// DialContext dials one of the ready pods behind this service forwarder.
//
// As an approximation to load balancing, a random ready pod will be chosen for each dialing attempt.
func (f *serviceForwarder) DialContext(ctx context.Context) (net.Conn, error) {
	_, servicePortStr, err := net.SplitHostPort(f.addr)
	if err != nil {
		return nil, err
	}
	servicePort, err := strconv.Atoi(servicePortStr)
	if err != nil {
		return nil, err
	}

	service := v1.Service{}
	serviceObjectKey := types.NamespacedName{Namespace: f.namespace, Name: f.serviceName}

	if err := f.client.Get(ctx, serviceObjectKey, &service); err != nil {
		return nil, err
	}

	// TODO: support named ports? how it's supposed to work is not quite clear atm, and we don't use it ourselves
	// so this is deferred to later
	targetPort := intstr.FromInt(servicePort)
	for _, port := range service.Spec.Ports {
		if port.Port == int32(servicePort) {
			targetPort = port.TargetPort
		}
	}

	endpoints := v1.Endpoints{}

	if err := f.client.Get(ctx, serviceObjectKey, &endpoints); err != nil {
		return nil, err
	}

	var podTargets []*v1.ObjectReference
	for _, subset := range endpoints.Subsets {
		foundPort := false
		for _, port := range subset.Ports {
			foundPort = port.Port == int32(targetPort.IntValue())
			if foundPort {
				break
			}
		}
		if !foundPort {
			continue
		}

		for _, address := range subset.Addresses {
			if address.TargetRef.Kind == "Pod" {
				podTargets = append(podTargets, address.TargetRef)
			}
		}
	}

	if len(podTargets) == 0 {
		return nil, errors.New("no pod addresses found in service endpoints")
	}

	pod := podTargets[rand.Intn(len(podTargets))]

	// this should match a supported format of parsePodAddr(addr string)
	podAddr := fmt.Sprintf("%s.%s.pod.cluster.local:%s", pod.Name, pod.Namespace, targetPort.String())
	forwarder, err := f.store.GetOrCreateForwarder(f.network, podAddr, f.podForwarderFactory)
	if err != nil {
		return nil, err
	}

	return forwarder.DialContext(ctx)
}
