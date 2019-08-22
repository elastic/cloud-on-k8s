// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package test

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/dev/portforward"
)

// NewHTTPClient creates a new HTTP client that is aware of any port forwarding configuration.
func NewHTTPClient(caCerts []*x509.Certificate) *http.Client {
	client := http.Client{
		Timeout: 60 * time.Second,
	}

	transport := http.Transport{}
	if Ctx().AutoPortForwarding {
		transport.DialContext = portforward.NewForwardingDialer().DialContext
	}

	certPool := x509.NewCertPool()
	for _, c := range caCerts {
		certPool.AddCert(c)
	}
	transport.TLSClientConfig = &tls.Config{
		RootCAs: certPool,
	}
	client.Transport = &transport
	return &client
}
