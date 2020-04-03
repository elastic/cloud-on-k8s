// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package common

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"

	"go.elastic.co/apm/module/apmelasticsearch"

	"github.com/elastic/cloud-on-k8s/pkg/utils/cryptutil"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
)

// HTTPClient returns an http.Client configured for targeting a service managed by ECK.
// Features:
// - use the custom dialer if provided (can be nil) for eg. custom port-forwarding
// - use the provided ca certs for TLS verification (can be nil)
// - verify TLS certs, but ignore the server name: users may provide their own TLS certificate that may not
// match Kubernetes internal service name, but only the user-facing public endpoint
// - set APM spans with each request
func HTTPClient(dialer net.Dialer, caCerts []*x509.Certificate) *http.Client {
	certPool := x509.NewCertPool()
	for _, c := range caCerts {
		certPool.AddCert(c)
	}

	transportConfig := http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: certPool,

			// We use our own certificate verification because we permit users to provide their own certificates, which may not
			// be valid for the k8s service URL (though our self-signed certificates are). For instance, users may use a certificate
			// issued by a public CA. We opt to skip verifying here since we're not validating based on DNS names
			// or IP addresses, which means we have to do our own verification in VerifyPeerCertificate instead.

			// go requires either ServerName or InsecureSkipVerify (or both) when handshaking as a client since 1.3:
			// https://github.com/golang/go/commit/fca335e91a915b6aae536936a7694c4a2a007a60
			InsecureSkipVerify: true, // nolint
			VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
				return errors.New("tls: verify peer certificate not setup")
			},
		},
	}

	transportConfig.TLSClientConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		if verifiedChains != nil {
			return errors.New("tls: non-nil verifiedChains argument breaks crypto/tls.Config.VerifyPeerCertificate contract")
		}
		_, _, err := cryptutil.VerifyCertificateExceptServerName(rawCerts, transportConfig.TLSClientConfig)
		return err
	}

	// use the custom dialer if provided
	if dialer != nil {
		transportConfig.DialContext = dialer.DialContext
	}

	return &http.Client{Transport: apmelasticsearch.WrapRoundTripper(&transportConfig)}
}
