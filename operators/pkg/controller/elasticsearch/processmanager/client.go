// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package processmanager

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/cryptutil"

	"github.com/elastic/cloud-on-k8s/operators/pkg/controller/elasticsearch/keystore"
	"github.com/elastic/cloud-on-k8s/operators/pkg/utils/net"
)

// DefaultReqTimeout is the default timeout of an HTTP request to the Process Manager
const DefaultReqTimeout = 1 * time.Minute

type Client interface {
	Start(ctx context.Context) (ProcessStatus, error)
	Stop(ctx context.Context) (ProcessStatus, error)
	Kill(ctx context.Context) (ProcessStatus, error)
	Status(ctx context.Context) (ProcessStatus, error)
	KeystoreStatus(ctx context.Context) (keystore.Status, error)
	Close()
}

type DefaultClient struct {
	Endpoint  string
	caCerts   []*x509.Certificate
	HTTP      *http.Client
	transport *http.Transport
}

// Close idle connections in the underlying http client.
func (c *DefaultClient) Close() {
	if c.transport != nil {
		// When the http transport goes out of scope, the underlying goroutines responsible
		// for handling keep-alive connections are not closed automatically.
		// Since this client gets recreated frequently we would effectively be leaking goroutines.
		// Let's make sure this does not happen by closing idle connections.
		c.transport.CloseIdleConnections()
	}
}

func NewClient(endpoint string, caCerts []*x509.Certificate, dialer net.Dialer) Client {
	var transportConfig http.Transport
	client := http.DefaultClient
	if len(caCerts) > 0 {
		certPool := x509.NewCertPool()
		for _, c := range caCerts {
			certPool.AddCert(c)
		}

		transportConfig = http.Transport{
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

		client = &http.Client{
			Transport: &transportConfig,
		}
	}

	return &DefaultClient{
		endpoint,
		caCerts,
		client,
		&transportConfig,
	}
}

func (c *DefaultClient) Start(ctx context.Context) (ProcessStatus, error) {
	var status ProcessStatus
	err := c.doRequest(ctx, "GET", "/es/start", &status)
	return status, err
}

func (c *DefaultClient) Stop(ctx context.Context) (ProcessStatus, error) {
	uri := "/es/stop"
	var status ProcessStatus
	err := c.doRequest(ctx, "GET", uri, &status)
	return status, err
}

func (c *DefaultClient) Kill(ctx context.Context) (ProcessStatus, error) {
	uri := "/es/kill"
	var status ProcessStatus
	err := c.doRequest(ctx, "GET", uri, &status)
	return status, err
}

func (c *DefaultClient) Status(ctx context.Context) (ProcessStatus, error) {
	var status ProcessStatus
	err := c.doRequest(ctx, "GET", "/es/status", &status)
	return status, err
}

func (c *DefaultClient) KeystoreStatus(ctx context.Context) (keystore.Status, error) {
	var status keystore.Status
	err := c.doRequest(ctx, "GET", "/keystore/status", &status)
	return status, err
}

func (c *DefaultClient) doRequest(ctx context.Context, method string, uri string, respBody interface{}) error {
	url := c.Endpoint + uri
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return err
	}

	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Try to unmarshal the response anyway
		_ = json.Unmarshal(body, respBody)

		return fmt.Errorf("%s %s failed, status: %d, body: %s", method, url, resp.StatusCode, string(body))
	}

	err = json.Unmarshal(body, respBody)
	if err != nil {
		return err
	}

	return nil
}
