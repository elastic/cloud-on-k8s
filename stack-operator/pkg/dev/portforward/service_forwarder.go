package portforward

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// serviceForwarder forwards one port of a service
type serviceForwarder struct {
	network, addr          string
	serviceName, namespace string

	client client.Client

	sync.Mutex
	forwarders map[string]Forwarder

	// podForwarderFactory enables injecting a custom forwarder factory in tests
	podForwarderFactory PodForwarderFactory
}

var _ Forwarder = &serviceForwarder{}

// PodForwarderFactory is a function that can produce forwarders
type PodForwarderFactory interface {
	NewPodForwarder(network, addr string) Forwarder
}

// ForwarderFactoryFunc is a converter from a function to a ForwarderFactory
type PodForwarderFactoryFunc func(network, addr string) Forwarder

func (f PodForwarderFactoryFunc) NewPodForwarder(network, addr string) Forwarder {
	return f(network, addr)
}

// defaultPodForwarderFactory is the default pod forwarder factory used outside of tests
var defaultPodForwarderFactory = PodForwarderFactoryFunc(func(network, addr string) Forwarder {
	return NewPodForwarder(network, addr)
})

// NewServiceForwarder returns a new initialized service forwarder
func NewServiceForwarder(client client.Client, network, addr string) *serviceForwarder {
	serviceName, namespace := parseServiceAddr(addr)

	return &serviceForwarder{
		network: network,
		addr:    addr,

		client: client,

		serviceName: serviceName,
		namespace:   namespace,

		forwarders:          make(map[string]Forwarder),
		podForwarderFactory: defaultPodForwarderFactory,
	}
}

// parseServiceAddr parses the service name and namespace from a connection address
func parseServiceAddr(addr string) (string, string) {
	// services generally look like this (as FQDN): {name}.{namespace}.svc.cluster.local
	parts := strings.SplitN(addr, ".", 3)
	name := parts[0]
	namespace := parts[1]
	return name, namespace
}

// Run starts the service forwarder, blocking until it's done
func (f *serviceForwarder) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// DialContext dials one of the ready pods behind this service forwarder.
//
// As an approximation to load balancing, a random ready pod will be chosen for each dial.
func (f *serviceForwarder) DialContext(ctx context.Context) (net.Conn, error) {
	service := v1.Service{}
	serviceObjectKey := types.NamespacedName{Namespace: f.namespace, Name: f.serviceName}

	if err := f.client.Get(ctx, serviceObjectKey, &service); err != nil {
		return nil, err
	}

	podList := v1.PodList{}
	podListOptions := client.ListOptions{
		LabelSelector: labels.Set(service.Spec.Selector).AsSelector(),
	}

	if err := f.client.List(ctx, &podListOptions, &podList); err != nil {
		return nil, err
	}

	readyPods := readyPods(podList.Items)

	if len(readyPods) == 0 {
		return nil, errors.New("no pods ready")
	}

	_, servicePortStr, err := net.SplitHostPort(f.addr)
	if err != nil {
		return nil, err
	}
	servicePort, err := strconv.Atoi(servicePortStr)
	if err != nil {
		return nil, err
	}

	targetPort := intstr.FromInt(servicePort)
	for _, port := range service.Spec.Ports {
		if port.Port == int32(servicePort) {
			targetPort = port.TargetPort
		}
	}

	pod := readyPods[rand.Intn(len(readyPods))]

	podAddr := fmt.Sprintf("%s.%s.pod.cluster.local:%s", pod.Name, pod.Namespace, targetPort.String())
	forwarder := f.getOrCreateForwarder(f.network, podAddr)

	return forwarder.DialContext(ctx)
}

// getOrCreateForwarder returns a cached
func (f *serviceForwarder) getOrCreateForwarder(network, addr string) Forwarder {
	f.Lock()
	defer f.Unlock()

	key := addr

	fwd, ok := f.forwarders[key]
	if !ok {
		fwd = NewPodForwarder(network, addr)
		f.forwarders[key] = fwd

		// start the podForwarder in a goroutine
		go func() {
			// remove the podForwarder from the map when done running
			defer func() {
				f.Lock()
				defer f.Unlock()

				delete(f.forwarders, key)
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

// readyPods returns a new slice that containing only the pods that are ready.
func readyPods(pods []v1.Pod) []v1.Pod {
	var readyPods []v1.Pod
	for _, pod := range pods {
		conditionsTrue := 0
		for _, cond := range pod.Status.Conditions {
			if cond.Status == v1.ConditionTrue && (cond.Type == v1.ContainersReady || cond.Type == v1.PodReady) {
				conditionsTrue++
			}
		}
		if conditionsTrue == 2 {
			readyPods = append(readyPods, pod)
		}
	}
	return readyPods
}
