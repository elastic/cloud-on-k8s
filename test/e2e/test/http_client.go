// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package test

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"time"

	"github.com/elastic/cloud-on-k8s/v2/pkg/dev/portforward"
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

	//nolint:gosec  // [G402: TLS MinVersion too low] is not a concern here as it is test code.
	transport.TLSClientConfig = &tls.Config{
		RootCAs: certPool,
	}
	client.Transport = &transport
	return &client
}
