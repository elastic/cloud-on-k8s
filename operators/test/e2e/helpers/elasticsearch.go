// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package helpers

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"

	elasticsearch "github.com/elastic/go-elasticsearch"
	"github.com/elastic/k8s-operators/operators/pkg/apis/elasticsearch/v1alpha1"
	"github.com/elastic/k8s-operators/operators/pkg/controller/common/version"
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch/client"
	"github.com/elastic/k8s-operators/operators/pkg/dev/portforward"
	"github.com/elastic/k8s-operators/operators/pkg/utils/net"
)

// define an alias to avoid some name conflict
type APIClient = elasticsearch.Client

// Client is a Elasticsearch client that satisfies the Client interface used in the operator but also
// come along with a native API client which is more appropriate for creating indices and loading some data
// with the bulk api.
type Client struct {
	client.Client
	*APIClient
}

// NewElasticsearchClient returns an ES client for the given stack's ES cluster
func NewElasticsearchClient(es v1alpha1.Elasticsearch, k *K8sHelper) (*Client, error) {
	password, err := k.GetElasticPassword(es.Name)
	if err != nil {
		return nil, err
	}
	inClusterURL := fmt.Sprintf("https://%s-es.%s.svc.cluster.local:9200", es.Name, es.Namespace)

	// Get the certificate authority
	caCerts, err := k.GetCACert(es.Name)
	if err != nil {
		return nil, err
	}
	certPool := x509.NewCertPool()
	for _, c := range caCerts {
		certPool.AddCert(c)
	}
	transportConfig := http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: certPool,
		},
	}

	// use the custom dialer if provided
	var dialer net.Dialer
	if *autoPortForward {
		dialer = portforward.NewForwardingDialer()
	}
	if dialer != nil {
		transportConfig.DialContext = dialer.DialContext
	}

	// Create the configuration of the API client
	config := elasticsearch.Config{
		Username:  "elastic",
		Password:  password,
		Transport: &transportConfig,
		Addresses: []string{inClusterURL},
	}
	// Create the API client
	apiClient, err := elasticsearch.NewClient(config)
	if err != nil {
		return nil, err
	}

	// Create the operator client
	v, err := version.Parse(es.Spec.Version)
	if err != nil {
		return nil, err
	}
	opClient := client.NewElasticsearchClient(
		dialer,
		inClusterURL,
		client.UserAuth{Name: "elastic", Password: password},
		*v,
		caCerts,
	)
	return &Client{opClient, apiClient}, nil
}
