// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package portforward

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	netutils "k8s.io/utils/net"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const syntheticDNSSegment = "pod"

// ServiceForwarder forwards one port of a service
type ServiceForwarder struct {
	network, addr string
	serviceNSN    types.NamespacedName

	// client is used to look up the service and pods selected by the service during dialing
	client client.Client

	store *ForwarderStore

	// podForwarderFactory enables injecting a custom forwarder factory in tests
	podForwarderFactory ForwarderFactory
}

var _ Forwarder = &ServiceForwarder{}

// defaultPodForwarderFactory is the default pod forwarder factory used outside of tests
var defaultPodForwarderFactory = ForwarderFactory(func(ctx context.Context, network, addr string) (Forwarder, error) {
	clientset, err := newDefaultKubernetesClientset()
	if err != nil {
		return nil, err
	}
	return NewPodForwarder(ctx, network, addr, clientset)
})

// NewServiceForwarder returns a new initialized service forwarder
func NewServiceForwarder(client client.Client, network, addr string) (*ServiceForwarder, error) {
	serviceNSN, err := parseServiceAddr(addr)
	if err != nil {
		return nil, err
	}

	return &ServiceForwarder{
		network: network,
		addr:    addr,

		client: client,

		serviceNSN: *serviceNSN,

		store:               NewForwarderStore(),
		podForwarderFactory: defaultPodForwarderFactory,
	}, nil
}

// parseServiceAddr parses the service name and namespace from a connection address
func parseServiceAddr(addr string) (*types.NamespacedName, error) {
	// services generally look like this (as FQDN): {name}.{namespace}.svc
	parts := strings.SplitN(addr, ".", 3)

	if len(parts) <= 2 {
		return nil, fmt.Errorf("unsupported service address format: %s", addr)
	}

	return &types.NamespacedName{Namespace: parts[1], Name: parts[0]}, nil
}

// Run starts the service forwarder, blocking until it's done
func (f *ServiceForwarder) Run(ctx context.Context) error {
	// TODO: /could/ consider snipping connections here when pods turn unready, but that does not match the default
	// Service behavior
	<-ctx.Done()
	return nil
}

// DialContext dials one of the ready pods behind this service forwarder.
//
// As an approximation to load balancing, a random ready pod will be chosen for each dialing attempt.
func (f *ServiceForwarder) DialContext(ctx context.Context) (net.Conn, error) {
	_, servicePortStr, err := net.SplitHostPort(f.addr)
	if err != nil {
		return nil, err
	}
	servicePort, err := netutils.ParsePort(servicePortStr, false)
	if err != nil {
		return nil, err
	}

	service := corev1.Service{}

	if err := f.client.Get(ctx, f.serviceNSN, &service); err != nil {
		return nil, err
	}

	// TODO: support named ports? how it's supposed to work is not quite clear atm, and we don't use it ourselves
	// so this is deferred to later

	targetPort := intstr.FromInt(0)
	for _, port := range service.Spec.Ports {
		if port.Port == int32(servicePort) {
			// default to using the same port between the service and the target
			targetPort = intstr.FromInt(int(port.Port))

			// if .TargetPort is non-0, we use that
			if port.TargetPort.IntValue() != 0 {
				targetPort = port.TargetPort
			}
			break
		}
	}

	if targetPort.IntValue() == 0 {
		return nil, fmt.Errorf("service is not listening on port: %d", servicePort)
	}

	endpoints := corev1.Endpoints{}

	if err := f.client.Get(ctx, f.serviceNSN, &endpoints); err != nil {
		return nil, err
	}

	var podTargets []*corev1.ObjectReference
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

	pod := podTargets[rand.Intn(len(podTargets))] //nolint:gosec

	// this should match a supported format of parsePodAddr(addr string)
	podAddr := fmt.Sprintf("%s.%s.%s:%s", pod.Name, pod.Namespace, syntheticDNSSegment, targetPort.String())
	forwarder, err := f.store.GetOrCreateForwarder(f.network, podAddr, f.podForwarderFactory)
	if err != nil {
		return nil, err
	}

	return forwarder.DialContext(ctx)
}
