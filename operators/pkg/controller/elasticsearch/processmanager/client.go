// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package processmanager

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/elastic/k8s-operators/operators/pkg/utils/net"
)

const DefaultReqTimeout = 1 * time.Minute

type Client interface {
	Start(ctx context.Context) (ProcessStatus, error)
	Stop(ctx context.Context) (ProcessStatus, error)
	Kill(ctx context.Context) (ProcessStatus, error)
	Status(ctx context.Context) (ProcessStatus, error)
}

type DefaultClient struct {
	Endpoint string
	caCerts  []*x509.Certificate
	HTTP     *http.Client
}

func NewClient(endpoint string, caCerts []*x509.Certificate, dialer net.Dialer) Client {
	client := http.DefaultClient
	if len(caCerts) > 0 {
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
