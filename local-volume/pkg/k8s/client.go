package k8s

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

type Client struct {
	ClientSet kubernetes.Interface
}

// NewClient creates a k8s in-cluster client
func NewClient() (*Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &Client{ClientSet: clientSet}, nil
}

// NewTestClient returns a stub client implementation with
// with the given objects pre-existing
func NewTestClient(k8sObj ...runtime.Object) *Client {
	return &Client{
		ClientSet: fake.NewSimpleClientset(k8sObj...),
	}
}
