// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/version"
	"github.com/elastic/cloud-on-k8s/pkg/utils/cryptutil"
	"github.com/elastic/cloud-on-k8s/pkg/utils/net"
)

type defaultClient struct {
	esVersion version.Version
	endpoint  string
	userAuth  UserAuth
	transport *http.Transport
	caCerts   []*x509.Certificate
	*elasticsearchClients
}

// NewDefaultElasticsearchClient creates a new client for the target cluster.
//
// If dialer is not nil, it will be used to create new TCP connections
func NewDefaultElasticsearchClient(
	dialer net.Dialer,
	esURL string,
	esUser UserAuth,
	v version.Version,
	caCerts []*x509.Certificate,
) (Client, error) {
	certPool := x509.NewCertPool()
	for _, c := range caCerts {
		certPool.AddCert(c)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: certPool,

			// go requires either ServerName or InsecureSkipVerify (or both) when handshaking as a client since 1.3:
			// https://github.com/golang/go/commit/fca335e91a915b6aae536936a7694c4a2a007a60
			// we opt to skip verifying here because we're not validating based on DNS names or IP addresses, which means
			// we have to do our verification in the VerifyPeerCertificate instead.
			InsecureSkipVerify: true,
			VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
				return errors.New("tls: verify peer certificate not setup")
			},
		},
	} // #nosec G402

	transport.TLSClientConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		if verifiedChains != nil {
			return errors.New("tls: non-nil verifiedChains argument breaks crypto/tls.Config.VerifyPeerCertificate contract")
		}
		_, _, err := cryptutil.VerifyCertificateExceptServerName(rawCerts, transport.TLSClientConfig)
		return err
	}

	// use the custom dialer if provided
	if dialer != nil {
		transport.DialContext = dialer.DialContext
	}

	es, err := newElasticsearchClients(v, esURL, esUser, transport)
	if err != nil {
		return nil, err
	}

	base := &defaultClient{
		esVersion:            v,
		endpoint:             esURL,
		userAuth:             esUser,
		caCerts:              caCerts,
		transport:            transport,
		elasticsearchClients: es,
	}
	return base, nil
}

// Close idle connections in the underlying http client.
// Should be called once this client is not used anymore.
func (c *defaultClient) Close() {
	if c.transport != nil {
		// When the http transport goes out of scope, the underlying goroutines responsible
		// for handling keep-alive connections are not closed automatically.
		// Since this client gets recreated frequently we would effectively be leaking goroutines.
		// Let's make sure this does not happen by closing idle connections.
		c.transport.CloseIdleConnections()
	}
}

func (c *defaultClient) Equal(other Client) bool {
	switch other := other.(type) {
	case *defaultClient:
		// handle nil case
		if other == nil && c != nil {
			return false
		}
		// compare ca certs
		if len(c.caCerts) != len(other.caCerts) {
			return false
		}
		for i := range c.caCerts {
			if !c.caCerts[i].Equal(other.caCerts[i]) {
				return false
			}
		}

		// compare versions:
		if c.esVersion != other.esVersion {
			return false
		}

		// compare endpoint and user creds
		return c.endpoint == other.endpoint && c.userAuth == other.userAuth
	default:
		return false
	}
}

func (c *defaultClient) Request(ctx context.Context, r *http.Request) (*http.Response, error) {
	return c.perform(r.WithContext(ctx))
}
